// Command florilegium is an MCP server that surfaces one apt item at a time
// from a user-supplied corpus, recency-aware and without recent repeats. This
// is the bootstrap entrypoint: it constructs the server and speaks MCP over
// stdio but registers no tools yet — the corpus, history, and tool contract
// land in subsequent issues.
package main

import (
	"context"
	"log"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	serverName    = "florilegium"
	serverVersion = "0.1.0"
)

// newServer constructs the florilegium MCP server. It is factored out of main
// so construction is unit-testable without standing up a live stdio transport.
func newServer() *mcp.Server {
	return mcp.NewServer(
		&mcp.Implementation{Name: serverName, Version: serverVersion},
		nil,
	)
}

func main() {
	if err := newServer().Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		log.Fatalf("mcp server: %v", err)
	}
}
