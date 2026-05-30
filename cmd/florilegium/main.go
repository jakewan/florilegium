// Command florilegium is an MCP server that surfaces one apt item at a time
// from a user-supplied corpus, recency-aware and without recent repeats. This
// is the bootstrap entrypoint: it loads config, the corpus, and the history
// store, constructs the corpus-backed server, and speaks MCP over stdio.
package main

import (
	"context"
	"errors"
	"io"
	"log"

	"github.com/jakewan/florilegium/internal/config"
	"github.com/jakewan/florilegium/internal/corpus"
	"github.com/jakewan/florilegium/internal/history"
	"github.com/jakewan/florilegium/internal/server"
	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// codeServerClosing is the SDK's JSON-RPC error code for a connection torn
// down because the server is shutting down — the SDK's internal
// jsonrpc2.ErrServerClosing, which surfaces to callers as a *jsonrpc.Error
// carrying this code. It is not among the public jsonrpc package's exported
// standard codes, so it is named here. Matching the code rather than the
// message text keeps clean-shutdown detection stable across SDK upgrades
// that reword the message.
const codeServerClosing = -32004

// run serves a session over the given transport, returning nil when the
// session ends normally (the client disconnects) and an error only on a
// genuine failure. It is factored out of main so the shutdown classification
// is testable over an in-process transport.
func run(ctx context.Context, srv *mcp.Server, transport mcp.Transport) error {
	if err := srv.Run(ctx, transport); err != nil && !isCleanShutdown(err) {
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

	// Load config, corpus, and history before serving so any misconfiguration
	// fails fast with a clear message instead of surfacing mid-session. Each
	// failure is fatal here — main is the one place a fatal is acceptable.
	cfg, err := config.Load(ctx)
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	c, err := corpus.Load(ctx, cfg.Corpus)
	if err != nil {
		log.Fatalf("corpus: %v", err)
	}
	historyPath, err := history.DefaultPath()
	if err != nil {
		log.Fatalf("history: %v", err)
	}
	srv := server.New(c, history.New(historyPath), cfg.RecencyWindow)

	if err := run(ctx, srv, &mcp.StdioTransport{}); err != nil {
		log.Fatalf("mcp server: %v", err)
	}
}
