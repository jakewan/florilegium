// Package history is the durable memory of which corpus items were recently
// surfaced, so the server can enforce the recency window across sessions. It is
// a flat, append-only JSON-lines log keyed by item id: recording a use appends
// one line, and eligibility is a tail-read of the most recent entries.
//
// The flat-file shape is deliberate. The model is an ordered log and the only
// query is "the last N picks," so a SQL engine and migrations would be weight
// without benefit for a daemonless, single-process-per-session tool. A
// single-line O_APPEND write is atomic on POSIX, so concurrent sessions append
// without interleaving.
package history

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// entry is one recorded use as stored on disk. At is informational — recency is
// determined by append order, not the timestamp — but it makes the log readable
// and leaves room for a time-based window later without a format change.
type entry struct {
	ID string `json:"id"`
	At string `json:"at"`
}

// Store is an append-only log of item uses backing the recency window. It holds
// only the file path; no file is opened until Record or Eligible runs, so an
// absent file is a valid empty history rather than a construction error.
type Store struct {
	path string
}

// New returns a Store backed by the JSON-lines file at path.
func New(path string) *Store {
	return &Store{path: path}
}

// Record appends a use of id to the log, creating the parent directory and file
// on first write. The entry is written as a single JSON line; an O_APPEND write
// of one short line is atomic, so concurrent sessions do not corrupt each other.
func (s *Store) Record(_ context.Context, id string) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("creating history dir: %w", err)
	}

	line, err := json.Marshal(entry{ID: id, At: time.Now().UTC().Format(time.RFC3339)})
	if err != nil {
		return fmt.Errorf("encoding history entry: %w", err)
	}

	f, err := os.OpenFile(s.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("opening history %s: %w", s.path, err)
	}

	// Close is checked explicitly rather than deferred: on a write, a Close
	// error can be the first sign the bytes never reached disk, so it must not
	// be swallowed.
	_, writeErr := f.Write(append(line, '\n'))
	closeErr := f.Close()
	if writeErr != nil {
		return fmt.Errorf("appending to history %s: %w", s.path, writeErr)
	}
	if closeErr != nil {
		return fmt.Errorf("closing history %s: %w", s.path, closeErr)
	}
	return nil
}

// Eligible returns the candidate ids not used within the last window recorded
// uses, preserving the input order. A non-positive window, an absent log, or an
// empty log leaves every candidate eligible.
func (s *Store) Eligible(_ context.Context, candidateIDs []string, window int) ([]string, error) {
	recent, err := s.recent(window)
	if err != nil {
		return nil, err
	}

	var eligible []string
	for _, id := range candidateIDs {
		if _, used := recent[id]; !used {
			eligible = append(eligible, id)
		}
	}
	return eligible, nil
}

// recent returns the set of ids appearing in the last window entries of the log.
// A missing file yields an empty set so a first run treats everything as
// eligible. Blank or unparseable lines are skipped defensively — a partially
// written trailing line from a crash should not fail the whole read.
func (s *Store) recent(window int) (map[string]struct{}, error) {
	ids := make(map[string]struct{})
	if window <= 0 {
		return ids, nil
	}

	f, err := os.Open(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return ids, nil
		}
		return nil, fmt.Errorf("reading history %s: %w", s.path, err)
	}
	var order []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		raw := scanner.Bytes()
		if len(raw) == 0 {
			continue
		}
		var e entry
		if err := json.Unmarshal(raw, &e); err != nil || e.ID == "" {
			continue
		}
		order = append(order, e.ID)
	}
	// Close after scanning, preferring the scan error if both fail — a read
	// Close error is unlikely but should not be silently dropped under the
	// repo's check-blank errcheck.
	scanErr := scanner.Err()
	if closeErr := f.Close(); closeErr != nil && scanErr == nil {
		scanErr = closeErr
	}
	if scanErr != nil {
		return nil, fmt.Errorf("reading history %s: %w", s.path, scanErr)
	}

	start := max(0, len(order)-window)
	for _, id := range order[start:] {
		ids[id] = struct{}{}
	}
	return ids, nil
}

// DefaultPath resolves $XDG_STATE_HOME/florilegium/history.jsonl, falling back
// to ~/.local/state when the variable is unset or empty. It reads the env var
// directly (rather than os.UserCacheDir and friends) so the path matches the
// documented XDG layout on every platform and stays test-isolable via t.Setenv.
func DefaultPath() (string, error) {
	base := os.Getenv("XDG_STATE_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolving state dir: %w", err)
		}
		base = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(base, "florilegium", "history.jsonl"), nil
}
