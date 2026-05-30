package server

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestNewConstructs is the bootstrap smoke test: the server must construct
// without panicking — which also exercises the list_candidates input-schema
// build and its exclude_recent default wiring.
func TestNewConstructs(t *testing.T) {
	if New(fixtureCorpus(), newTestStore(t), 2) == nil {
		t.Fatal("New() returned nil")
	}
}

// TestEnumeratesRegisteredTools connects a client over an in-memory transport
// and asserts the full tool contract comes back — the acceptance criterion that
// a connected client can discover the tools before calling them.
func TestEnumeratesRegisteredTools(t *testing.T) {
	ctx := context.Background()
	cs := connect(t, New(fixtureCorpus(), newTestStore(t), 2))

	res, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}

	got := make(map[string]bool, len(res.Tools))
	for _, tool := range res.Tools {
		got[tool.Name] = true
	}

	want := []string{"list_candidates", "record_use", "list_tags"}
	if len(res.Tools) != len(want) {
		t.Errorf("ListTools returned %d tools, want %d: %v", len(res.Tools), len(want), toolNames(res.Tools))
	}
	for _, name := range want {
		if !got[name] {
			t.Errorf("tool %q not enumerated; got %v", name, toolNames(res.Tools))
		}
	}
}

func toolNames(tools []*mcp.Tool) []string {
	names := make([]string, len(tools))
	for i, tool := range tools {
		names[i] = tool.Name
	}
	return names
}
