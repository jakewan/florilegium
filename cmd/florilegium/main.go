// Command florilegium is an MCP server that surfaces one apt item at a time
// from a user-supplied corpus, recency-aware and without recent repeats. This
// is the bootstrap entrypoint: it loads config, the corpus, and the history
// store, constructs the corpus-backed server, and speaks MCP over stdio.
package main

import (
	"context"
	"errors"
	"flag"
	"io"
	"log"
	"os"

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

// parseConfigFlag parses the --config flag from args, returning the path or ""
// when unset. It uses a local FlagSet rather than the global flag.CommandLine
// so the parsing is a pure function the test can drive directly. An unknown
// flag is an error rather than a silent no-op.
func parseConfigFlag(args []string) (string, error) {
	fs := flag.NewFlagSet("florilegium", flag.ContinueOnError)
	configPath := fs.String("config", "", "path to the config file (overrides $FLORILEGIUM_CONFIG and the XDG default)")
	if err := fs.Parse(args); err != nil {
		return "", err
	}
	return *configPath, nil
}

func main() {
	ctx := context.Background()

	configOverride, err := parseConfigFlag(os.Args[1:])
	if err != nil {
		log.Fatalf("flags: %v", err)
	}

	// Load config, corpus, and history before serving so any misconfiguration
	// fails fast with a clear message instead of surfacing mid-session. Each
	// failure is fatal here — main is the one place a fatal is acceptable.
	cfg, err := config.Load(ctx, configOverride)
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	c, err := corpus.Load(ctx, cfg.Corpus)
	if err != nil {
		log.Fatalf("corpus: %v", err)
	}
	// The config's history path wins when set, so an instance's rotation state
	// travels with its config; otherwise fall back to the XDG default. This is
	// where the default is resolved — config stays ignorant of the history
	// package, the same way corpus defaults live here, not in config.
	historyPath := cfg.History
	if historyPath == "" {
		historyPath, err = history.DefaultPath()
		if err != nil {
			log.Fatalf("history: %v", err)
		}
	}
	store := history.New(historyPath)

	// Trim the log once per session so it can't grow without bound across the life
	// of an install. retain stays well above the recency window (see Compact) so
	// trimming never changes an eligibility result; the floor keeps a useful tail
	// even for a tiny window. Unlike the loads above, a failed trim is non-fatal:
	// maintenance must not block serving, and the diagnostic goes to stderr because
	// stdout carries the JSON-RPC protocol stream.
	retain := max(cfg.RecencyWindow*10, 1000)
	if err := store.Compact(ctx, retain); err != nil {
		log.Printf("history compaction: %v", err)
	}

	srv := server.New(c, store, cfg.RecencyWindow)

	if err := run(ctx, srv, &mcp.StdioTransport{}); err != nil {
		log.Fatalf("mcp server: %v", err)
	}
}
