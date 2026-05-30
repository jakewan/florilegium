// Package server builds the florilegium MCP server: it owns the tool contract
// and the corpus-backed handlers behind it. The split of responsibility is
// deliberate — this server is pure mechanism. It holds the corpus, remembers
// which ids were used recently, serves candidates, and records picks; it
// applies no relevance logic. Deciding which candidate fits the moment is the
// calling agent's job.
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/jakewan/florilegium/internal/corpus"
	"github.com/jakewan/florilegium/internal/history"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	serverName    = "florilegium"
	serverVersion = "0.1.0"
)

// deps carries what the handlers need: the corpus, the recency store, the
// window size, and a by-id index built once at construction so candidate
// mapping and unknown-id checks are O(1) rather than a per-call scan.
type deps struct {
	corpus *corpus.Corpus
	store  *history.Store
	window int
	byID   map[string]corpus.Item
}

// listCandidatesInput is tag-filtered, recency-aware shortlist request. Every
// field is optional; an absent field takes the documented default. json tags
// without omitempty would make a field required, so each optional field carries
// it; jsonschema tags hold plain-prose descriptions only.
type listCandidatesInput struct {
	Tags          []string `json:"tags,omitempty"           jsonschema:"restrict to items sharing at least one of these tags"`
	Limit         int      `json:"limit,omitempty"          jsonschema:"maximum candidates to return; 0 or omitted means no cap"`
	ExcludeRecent bool     `json:"exclude_recent,omitempty" jsonschema:"exclude items used within the recency window; defaults to true"`
}

// candidate is one shortlist entry. Attribution and Tags are omitted when empty
// so the wire shape stays minimal for items that carry neither.
type candidate struct {
	ID          string   `json:"id"`
	Text        string   `json:"text"`
	Attribution string   `json:"attribution,omitempty"`
	Tags        []string `json:"tags,omitempty"`
}

type listCandidatesOutput struct {
	Candidates []candidate `json:"candidates"`
}

// recordUseInput names the item that was used. ID is required — no omitempty —
// so an omitted id fails schema validation before reaching the handler.
type recordUseInput struct {
	ID string `json:"id" jsonschema:"id of the item that was used"`
}

// recordUseOutput echoes the recorded id, giving the caller a structured
// confirmation of exactly which id the server persisted.
type recordUseOutput struct {
	ID string `json:"id"`
}

type listTagsOutput struct {
	Tags []string `json:"tags"`
}

// New builds the florilegium MCP server with the corpus-backed tool contract
// registered. c must be non-nil — the caller loads it via corpus.Load, which
// rejects an empty corpus, so the by-id index always has something to hold.
func New(c *corpus.Corpus, store *history.Store, window int) *mcp.Server {
	byID := make(map[string]corpus.Item, len(c.Items))
	for _, it := range c.Items {
		byID[it.ID] = it
	}
	d := &deps{corpus: c, store: store, window: window, byID: byID}

	s := mcp.NewServer(&mcp.Implementation{Name: serverName, Version: serverVersion}, nil)

	// list_candidates uses an explicit input schema so the true default for
	// exclude_recent is published in the tool contract rather than buried in Go.
	// The SDK injects the default when the caller omits the field, but only for
	// non-required properties — which is why the field is omitempty above. A
	// schema-build failure here is a programmer error in the struct, not runtime
	// input, so it panics rather than degrading a tool to no default.
	in, err := jsonschema.For[listCandidatesInput](nil)
	if err != nil {
		panic(fmt.Sprintf("server: building list_candidates input schema: %v", err))
	}
	in.Properties["exclude_recent"].Default = json.RawMessage("true")
	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_candidates",
		Description: "Return a shortlist of items (id, text, attribution, tags), excluding recently-used ones, for the caller to choose from.",
		InputSchema: in,
	}, d.listCandidates)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "record_use",
		Description: "Mark an item as used so it drops out of rotation for the recency window.",
	}, d.recordUse)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_tags",
		Description: "List the tags present in the corpus so the caller can narrow before choosing.",
	}, d.listTags)

	return s
}

// listCandidates returns corpus items in file order, optionally narrowed to a
// tag set, with recently-used ids excluded and the count capped. It applies no
// ranking: order is the corpus's own, and choosing among the result is the
// caller's job.
func (d *deps) listCandidates(ctx context.Context, _ *mcp.CallToolRequest, in listCandidatesInput) (*mcp.CallToolResult, listCandidatesOutput, error) {
	var ids []string
	for _, it := range d.corpus.Items {
		if matchesTags(it.Tags, in.Tags) {
			ids = append(ids, it.ID)
		}
	}

	if in.ExcludeRecent {
		eligible, err := d.store.Eligible(ctx, ids, d.window)
		if err != nil {
			return nil, listCandidatesOutput{}, fmt.Errorf("list_candidates: %w", err)
		}
		ids = eligible
	}

	if in.Limit > 0 && len(ids) > in.Limit {
		ids = ids[:in.Limit]
	}

	out := listCandidatesOutput{Candidates: make([]candidate, 0, len(ids))}
	for _, id := range ids {
		it := d.byID[id]
		out.Candidates = append(out.Candidates, candidate{
			ID:          it.ID,
			Text:        it.Text,
			Attribution: it.Attribution,
			Tags:        it.Tags,
		})
	}
	return nil, out, nil
}

// recordUse marks a corpus item used so it drops out of the recency window. An
// unknown id is rejected with an actionable error rather than silently appended
// — a pick the corpus does not contain is a caller bug, not a recordable event.
func (d *deps) recordUse(ctx context.Context, _ *mcp.CallToolRequest, in recordUseInput) (*mcp.CallToolResult, recordUseOutput, error) {
	id := strings.TrimSpace(in.ID)
	if id == "" {
		return nil, recordUseOutput{}, fmt.Errorf("record_use: empty id")
	}
	if _, ok := d.byID[id]; !ok {
		return nil, recordUseOutput{}, fmt.Errorf("record_use: unknown id %q", id)
	}
	if err := d.store.Record(ctx, id); err != nil {
		return nil, recordUseOutput{}, fmt.Errorf("record_use: %w", err)
	}
	return nil, recordUseOutput{ID: id}, nil
}

// listTags returns the distinct tags across the corpus, deduplicated and sorted,
// so the caller can narrow with list_candidates before choosing.
func (d *deps) listTags(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, listTagsOutput, error) {
	set := make(map[string]struct{})
	for _, it := range d.corpus.Items {
		for _, t := range it.Tags {
			set[t] = struct{}{}
		}
	}
	tags := make([]string, 0, len(set))
	for t := range set {
		tags = append(tags, t)
	}
	sort.Strings(tags)
	return nil, listTagsOutput{Tags: tags}, nil
}

// matchesTags reports whether an item with itemTags passes the filter. An empty
// filter matches everything; otherwise the match is OR — sharing any one tag is
// enough, leaving precise narrowing to the caller rather than forcing every tag.
func matchesTags(itemTags, filter []string) bool {
	if len(filter) == 0 {
		return true
	}
	for _, f := range filter {
		if slices.Contains(itemTags, f) {
			return true
		}
	}
	return false
}
