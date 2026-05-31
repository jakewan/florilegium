package server

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/jakewan/florilegium/internal/corpus"
	"github.com/jakewan/florilegium/internal/history"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// fixtureCorpus is a small, deterministic anthology used across the server
// tests: ids are stable, tags overlap so OR-filtering has something to resolve,
// and one item omits meta so optional-field handling is exercised.
func fixtureCorpus() *corpus.Corpus {
	return &corpus.Corpus{Items: []corpus.Item{
		{ID: "a", Text: "Alpha", Meta: map[string]string{"attribution": "Author X"}, Tags: []string{"focus"}},
		{ID: "b", Text: "Beta", Tags: []string{"calm"}},
		{ID: "c", Text: "Gamma", Meta: map[string]string{"attribution": "Author Y", "source": "Essays"}, Tags: []string{"focus", "calm"}},
	}}
}

// connect stands up the server over an in-memory transport and returns a
// connected client session, registering cleanup for both ends. It exists
// because every behavior test needs a live client/server pair; the wiring is
// identical across them.
func connect(t *testing.T, srv *mcp.Server) *mcp.ClientSession {
	t.Helper()
	ctx := context.Background()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()

	serverSession, err := srv.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	t.Cleanup(func() {
		if cerr := serverSession.Close(); cerr != nil {
			t.Errorf("server session close: %v", cerr)
		}
	})

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() {
		if cerr := clientSession.Close(); cerr != nil {
			t.Errorf("client session close: %v", cerr)
		}
	})
	return clientSession
}

// newTestStore returns a history store isolated under t.TempDir(), so recorded
// uses never touch the real XDG state directory.
func newTestStore(t *testing.T) *history.Store {
	t.Helper()
	return history.New(filepath.Join(t.TempDir(), "florilegium", "history.jsonl"))
}

// candidateIDs decodes a list_candidates result's structured output and returns
// the candidate ids in order, failing the test on a decode error or tool error.
func candidateIDs(t *testing.T, res *mcp.CallToolResult) []string {
	t.Helper()
	if res.IsError {
		t.Fatalf("list_candidates returned tool error: %s", errorText(res))
	}
	var out struct {
		Candidates []struct {
			ID string `json:"id"`
		} `json:"candidates"`
	}
	raw, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatalf("marshal structured content: %v", err)
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode list_candidates output: %v", err)
	}
	ids := make([]string, len(out.Candidates))
	for i, c := range out.Candidates {
		ids[i] = c.ID
	}
	return ids
}

// errorText pulls the human-readable message out of a tool-error result, where
// the SDK places it in the first text content block (not StructuredContent).
func errorText(res *mcp.CallToolResult) string {
	if len(res.Content) == 0 {
		return ""
	}
	if tc, ok := res.Content[0].(*mcp.TextContent); ok {
		return tc.Text
	}
	return ""
}

// TestAcceptanceListRecordList is the outside-in acceptance test for the whole
// contract: a client gets a shortlist, reports a pick, and the picked id drops
// out of the next shortlist within the recency window — the core promise of the
// server, exercised end-to-end over a real MCP session.
func TestAcceptanceListRecordList(t *testing.T) {
	ctx := context.Background()
	cs := connect(t, New(fixtureCorpus(), newTestStore(t), 2))

	first, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "list_candidates"})
	if err != nil {
		t.Fatalf("first list_candidates: %v", err)
	}
	if got := candidateIDs(t, first); !equalIDs(got, []string{"a", "b", "c"}) {
		t.Fatalf("first list_candidates = %v, want [a b c]", got)
	}

	rec, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "record_use",
		Arguments: map[string]any{"id": "a"},
	})
	if err != nil {
		t.Fatalf("record_use: %v", err)
	}
	if rec.IsError {
		t.Fatalf("record_use(a) returned tool error: %s", errorText(rec))
	}

	second, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "list_candidates"})
	if err != nil {
		t.Fatalf("second list_candidates: %v", err)
	}
	if got := candidateIDs(t, second); !equalIDs(got, []string{"b", "c"}) {
		t.Fatalf("second list_candidates = %v, want [b c] (a excluded after record_use)", got)
	}
}

// equalIDs compares two id slices for order-sensitive equality.
func equalIDs(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
