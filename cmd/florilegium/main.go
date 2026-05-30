// Command florilegium is an MCP server that surfaces one apt item at a time
// from a user-supplied corpus, recency-aware and without recent repeats. This
// is the bootstrap entrypoint: it loads config, registers the tool contract,
// and speaks MCP over stdio. Tool handlers are stubbed — the corpus, history,
// and real selection logic land in subsequent issues.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"

	"github.com/jakewan/florilegium/internal/config"
	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	serverName    = "florilegium"
	serverVersion = "0.1.0"

	// codeServerClosing is the SDK's JSON-RPC error code for a connection torn
	// down because the server is shutting down (jsonrpc2.ErrServerClosing). It
	// is not in the public jsonrpc package's exported standard codes, so it is
	// named here. Matching the code rather than the message text keeps clean-
	// shutdown detection stable across SDK upgrades that reword the message.
	codeServerClosing = -32004
)

// newServer constructs the florilegium MCP server with its tool contract
// registered. It is factored out of main so the server can be unit-tested
// without standing up a live stdio transport.
func newServer() *mcp.Server {
	s := mcp.NewServer(
		&mcp.Implementation{Name: serverName, Version: serverVersion},
		nil,
	)
	registerTools(s)
	return s
}

// registerTools registers the florilegium tool contract. Handlers are stubbed
// at this stage so a client can discover the tools; the corpus-backed behavior
// arrives with the corpus, history, and selection work.
func registerTools(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_candidates",
		Description: "Return a shortlist of items (id, text, attribution, tags), excluding recently-used ones, for the caller to choose from.",
	}, stub("list_candidates"))
	mcp.AddTool(s, &mcp.Tool{
		Name:        "record_use",
		Description: "Mark an item as used so it drops out of rotation for the recency window.",
	}, stub("record_use"))
	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_tags",
		Description: "List the tags present in the corpus so the caller can narrow before choosing.",
	}, stub("list_tags"))
}

// stub returns a not-yet-implemented handler for the named tool. The tool is
// still enumerable; calling it surfaces a clear tool error until the real
// handler lands.
func stub(name string) mcp.ToolHandlerFor[struct{}, struct{}] {
	return func(context.Context, *mcp.CallToolRequest, struct{}) (*mcp.CallToolResult, struct{}, error) {
		return nil, struct{}{}, fmt.Errorf("%s: not implemented", name)
	}
}

// run serves a session over the given transport, returning nil when the
// session ends normally (the client disconnects) and an error only on a
// genuine failure. It is factored out of main so the shutdown classification
// is testable over an in-process transport.
func run(ctx context.Context, transport mcp.Transport) error {
	if err := newServer().Run(ctx, transport); err != nil && !isCleanShutdown(err) {
		return err
	}
	return nil
}

// isCleanShutdown reports whether err is the routine end of a session rather
// than a failure: a nil error, the input stream reaching EOF, or the SDK's
// server-closing signal raised as in-flight calls are torn down on disconnect.
func isCleanShutdown(err error) bool {
	if err == nil || errors.Is(err, io.EOF) {
		return true
	}
	var wire *jsonrpc.Error
	return errors.As(err, &wire) && wire.Code == codeServerClosing
}

func main() {
	ctx := context.Background()

	// Validate config at startup so a missing or malformed file fails fast with
	// a clear message instead of surfacing later. The loaded values are consumed
	// by the corpus and selection work that lands in subsequent issues.
	if _, err := config.Load(ctx); err != nil {
		log.Fatalf("config: %v", err)
	}

	if err := run(ctx, &mcp.StdioTransport{}); err != nil {
		log.Fatalf("mcp server: %v", err)
	}
}
