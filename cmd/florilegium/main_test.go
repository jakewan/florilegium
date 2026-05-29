package main

import "testing"

// TestNewServerConstructs is the bootstrap smoke test: the server must
// construct without panicking. Behavior (config loading, tool enumeration)
// arrives with issue #1 and is deliberately out of scope here.
func TestNewServerConstructs(t *testing.T) {
	if newServer() == nil {
		t.Fatal("newServer() returned nil")
	}
}
