package server

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// callParams builds a CallToolParams for name with the given arguments (nil for
// no arguments). When args is nil the Arguments field is left unset rather than
// holding a typed nil map: a typed nil map marshals to "arguments": null (a
// non-empty interface defeats omitempty), whereas a real client omitting
// arguments sends no field at all — the case the no-args tests mean to cover.
func callParams(name string, args map[string]any) *mcp.CallToolParams {
	p := &mcp.CallToolParams{Name: name}
	if args != nil {
		p.Arguments = args
	}
	return p
}

// TestListCandidates drives list_candidates through a real client session and
// pins the contract's filtering, recency, and limit behavior. Recording happens
// on the same store the server holds, before connecting, so the recency cases
// see a populated history.
func TestListCandidates(t *testing.T) {
	tests := []struct {
		name   string
		record []string // ids recorded before listing (oldest first)
		window int
		args   map[string]any
		want   []string
	}{
		{name: "no filter no history", window: 2, want: []string{"a", "b", "c"}},
		{name: "tag focus", window: 2, args: map[string]any{"tags": []string{"focus"}}, want: []string{"a", "c"}},
		{name: "tag calm", window: 2, args: map[string]any{"tags": []string{"calm"}}, want: []string{"b", "c"}},
		{name: "multi tag is OR", window: 2, args: map[string]any{"tags": []string{"focus", "calm"}}, want: []string{"a", "b", "c"}},
		{name: "unknown tag yields none", window: 2, args: map[string]any{"tags": []string{"nope"}}, want: nil},
		{name: "limit caps the count", window: 2, args: map[string]any{"limit": 2}, want: []string{"a", "b"}},
		{name: "limit zero means no cap", window: 2, args: map[string]any{"limit": 0}, want: []string{"a", "b", "c"}},
		{name: "default excludes recent", record: []string{"a"}, window: 2, want: []string{"b", "c"}},
		{name: "exclude_recent false keeps recent", record: []string{"a"}, window: 2, args: map[string]any{"exclude_recent": false}, want: []string{"a", "b", "c"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			store := newTestStore(t)
			for _, id := range tt.record {
				if err := store.Record(ctx, id); err != nil {
					t.Fatalf("Record(%q): %v", id, err)
				}
			}
			cs := connect(t, New(fixtureCorpus(), store, tt.window))

			res, err := cs.CallTool(ctx, callParams("list_candidates", tt.args))
			if err != nil {
				t.Fatalf("CallTool: %v", err)
			}
			if got := candidateIDs(t, res); !equalIDs(got, tt.want) {
				t.Errorf("list_candidates(%v) = %v, want %v", tt.args, got, tt.want)
			}
		})
	}
}

// TestListCandidatesToleratesNullArguments guards a robustness edge: a client
// that sends the literal JSON null for arguments (rather than omitting the
// field) must be treated as "no arguments" — applying the documented defaults —
// not crash the session. The default-carrying input schema makes the SDK panic
// on a null arguments payload without the server's normalizing middleware.
func TestListCandidatesToleratesNullArguments(t *testing.T) {
	ctx := context.Background()
	cs := connect(t, New(fixtureCorpus(), newTestStore(t), 2))

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "list_candidates",
		Arguments: json.RawMessage("null"),
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if got := candidateIDs(t, res); !equalIDs(got, []string{"a", "b", "c"}) {
		t.Errorf("list_candidates(null args) = %v, want [a b c] (null treated as no args)", got)
	}
}

// TestListCandidatesRejectsNegativeLimit pins that a negative limit is invalid
// input rejected by the published schema bound, not silently treated as "no
// cap". 0 and omitted already mean unlimited, so a negative maximum is
// meaningless and must surface as an error rather than returning the whole
// corpus.
func TestListCandidatesRejectsNegativeLimit(t *testing.T) {
	ctx := context.Background()
	cs := connect(t, New(fixtureCorpus(), newTestStore(t), 2))

	res, err := cs.CallTool(ctx, callParams("list_candidates", map[string]any{"limit": -1}))
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !res.IsError {
		t.Fatalf("list_candidates(limit=-1) = no error, want a validation error")
	}
}

// TestRecordUse pins the record path: a known id succeeds and echoes back, an
// unknown or blank id fails with an actionable, caller-visible tool error
// rather than a silently appended bad entry.
func TestRecordUse(t *testing.T) {
	tests := []struct {
		name    string
		args    map[string]any
		wantErr string // substring of the tool-error text; "" means expect success
		wantID  string // echoed id on success
	}{
		{name: "known id", args: map[string]any{"id": "a"}, wantID: "a"},
		{name: "known id with surrounding space", args: map[string]any{"id": " a "}, wantID: "a"},
		{name: "unknown id", args: map[string]any{"id": "nope"}, wantErr: `unknown id "nope"`},
		{name: "blank id", args: map[string]any{"id": "   "}, wantErr: "empty id"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			cs := connect(t, New(fixtureCorpus(), newTestStore(t), 2))

			res, err := cs.CallTool(ctx, callParams("record_use", tt.args))
			if err != nil {
				t.Fatalf("CallTool: %v", err)
			}

			if tt.wantErr != "" {
				if !res.IsError {
					t.Fatalf("record_use(%v) = no error, want error containing %q", tt.args, tt.wantErr)
				}
				if msg := errorText(res); !strings.Contains(msg, tt.wantErr) {
					t.Errorf("record_use error = %q, want substring %q", msg, tt.wantErr)
				}
				return
			}

			if res.IsError {
				t.Fatalf("record_use(%v) unexpected tool error: %s", tt.args, errorText(res))
			}
			var out struct {
				ID string `json:"id"`
			}
			decodeResult(t, res.StructuredContent, &out)
			if out.ID != tt.wantID {
				t.Errorf("record_use echoed id = %q, want %q", out.ID, tt.wantID)
			}
		})
	}
}

// TestListTags asserts the distinct tags come back deduplicated and sorted —
// item c shares both tags, so the union is {calm, focus} in sorted order.
func TestListTags(t *testing.T) {
	ctx := context.Background()
	cs := connect(t, New(fixtureCorpus(), newTestStore(t), 2))

	res, err := cs.CallTool(ctx, callParams("list_tags", nil))
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("list_tags tool error: %s", errorText(res))
	}
	var out struct {
		Tags []string `json:"tags"`
	}
	decodeResult(t, res.StructuredContent, &out)
	if !equalIDs(out.Tags, []string{"calm", "focus"}) {
		t.Errorf("list_tags = %v, want [calm focus]", out.Tags)
	}
}

// decodeResult re-marshals a result's structured content and unmarshals it into
// v. The client receives StructuredContent as a generic map, so a round-trip
// through JSON is the version-stable way to land it in a typed value.
func decodeResult(t *testing.T, structured any, v any) {
	t.Helper()
	raw, err := json.Marshal(structured)
	if err != nil {
		t.Fatalf("marshal structured content: %v", err)
	}
	if err := json.Unmarshal(raw, v); err != nil {
		t.Fatalf("decode structured content: %v", err)
	}
}
