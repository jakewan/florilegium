package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"testing"
	"time"

	"github.com/jakewan/florilegium/internal/corpus"
	"github.com/jakewan/florilegium/internal/history"
	"github.com/jakewan/florilegium/internal/server"
	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// nopWriteCloser adapts an io.Writer to io.WriteCloser for IOTransport, whose
// Writer field requires a Close the test never needs to act on.
type nopWriteCloser struct{ io.Writer }

func (nopWriteCloser) Close() error { return nil }

// TestRunReturnsNilOnPeerDisconnect asserts the server treats a client
// disconnecting (the input stream reaching EOF with traffic in flight) as a
// normal end-of-session, not an error — so the process exits 0 rather than
// reporting routine shutdown as a failure. The pipe stands in for stdin; the
// server is built over a one-item corpus since the contract under test is the
// shutdown classification, not tool behavior.
func TestRunReturnsNilOnPeerDisconnect(t *testing.T) {
	ctx := context.Background()
	pr, pw := io.Pipe()
	transport := &mcp.IOTransport{Reader: pr, Writer: nopWriteCloser{io.Discard}}

	c := &corpus.Corpus{Items: []corpus.Item{{ID: "x", Text: "y"}}}
	srv := server.New(c, history.New(filepath.Join(t.TempDir(), "history.jsonl")), 1)

	done := make(chan error, 1)
	go func() { done <- run(ctx, srv, transport) }()

	handshake := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"test","version":"0"}}}` + "\n" +
		`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}` + "\n"
	if _, err := io.WriteString(pw, handshake); err != nil {
		t.Fatalf("write handshake: %v", err)
	}
	if err := pw.Close(); err != nil { // peer disconnects (EOF) mid-traffic
		t.Fatalf("close pipe: %v", err)
	}

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("run() after peer disconnect = %v, want nil", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("run() did not return after peer disconnect")
	}
}

// TestParseConfigFlag pins the --config flag parsing in isolation from the
// global flag set: an unset flag yields the empty string (so resolution falls
// through to FLORILEGIUM_CONFIG and the XDG default), an explicit value in
// either accepted form round-trips, and an unknown flag is an error rather than
// a silent no-op.
func TestParseConfigFlag(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    string
		wantErr bool
	}{
		{"no args", []string{}, "", false},
		{"separate value", []string{"--config", "/x/config.yml"}, "/x/config.yml", false},
		{"equals value", []string{"--config=/x/config.yml"}, "/x/config.yml", false},
		{"single dash", []string{"-config", "/x/config.yml"}, "/x/config.yml", false},
		{"unknown flag", []string{"--bogus"}, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseConfigFlag(tt.args)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseConfigFlag(%q) error = nil, want error", tt.args)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseConfigFlag(%q) unexpected error: %v", tt.args, err)
			}
			if got != tt.want {
				t.Errorf("parseConfigFlag(%q) = %q, want %q", tt.args, got, tt.want)
			}
		})
	}
}

// TestIsCleanShutdown pins which session-end errors count as routine shutdown
// (swallowed) versus genuine failures (propagated). The server-closing wire
// error is matched by its JSON-RPC code rather than message text so an SDK
// upgrade that rewords the message does not silently regress the exit status.
func TestIsCleanShutdown(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, true},
		{"eof", io.EOF, true},
		{"wrapped eof", fmt.Errorf("read: %w", io.EOF), true},
		{"server closing", &jsonrpc.Error{Code: codeServerClosing, Message: "server is closing"}, true},
		{"wrapped server closing", fmt.Errorf("run: %w", &jsonrpc.Error{Code: codeServerClosing, Message: "server is closing"}), true},
		{"internal wire error", &jsonrpc.Error{Code: -32603, Message: "internal error"}, false},
		{"generic error", errors.New("boom"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isCleanShutdown(tt.err); got != tt.want {
				t.Errorf("isCleanShutdown(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}
