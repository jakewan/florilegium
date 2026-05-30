// Package history is the durable memory of which corpus items were recently
// surfaced, so the server can enforce the recency window across sessions. It is
// a flat, append-only JSON-lines log keyed by item id: recording a use appends
// one line, and eligibility is decided from the most recent entries in the log.
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
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
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
//
// An empty or whitespace-only id is rejected: the reader skips such entries, so
// persisting one would be a silent no-op that still grows the log and masks the
// caller bug that produced it.
func (s *Store) Record(_ context.Context, id string) error {
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("recording use: empty id")
	}

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
// eligible.
//
// The reader keeps only the last window ids in a ring buffer rather than loading
// the whole log, so memory stays bounded by the window no matter how large the
// log grows. It uses bufio.Reader, not bufio.Scanner: Scanner caps a line at
// 64K and aborts the entire read with ErrTooLong on a longer one, which would
// defeat the intent of tolerating a single corrupt line. ReadString imposes no
// line cap, so a blank, over-long, or otherwise malformed line is skipped by
// the unmarshal guard rather than failing the read — a partially written
// trailing line from a crash should not lose the rest of the history.
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

	// ring holds the most recent ids, overwriting oldest-first once full, so at
	// most window ids are retained while scanning a log of any size.
	ring := make([]string, 0, window)
	next := 0
	reader := bufio.NewReader(f)
	var readErr error
	for {
		line, err := reader.ReadString('\n')
		if len(line) > 0 {
			var e entry
			if jsonErr := json.Unmarshal([]byte(line), &e); jsonErr == nil && e.ID != "" {
				if len(ring) < window {
					ring = append(ring, e.ID)
				} else {
					ring[next] = e.ID
					next = (next + 1) % window
				}
			}
		}
		if err != nil {
			if !errors.Is(err, io.EOF) {
				readErr = err
			}
			break
		}
	}

	// Close after reading, preferring the read error if both fail — a read Close
	// error is unlikely but should not be silently dropped under the repo's
	// check-blank errcheck.
	if closeErr := f.Close(); closeErr != nil && readErr == nil {
		readErr = closeErr
	}
	if readErr != nil {
		return nil, fmt.Errorf("reading history %s: %w", s.path, readErr)
	}

	for _, id := range ring {
		ids[id] = struct{}{}
	}
	return ids, nil
}

// Compact bounds the log so it cannot grow without limit across the life of an
// install. It keeps the most recent retain entries and drops the rest, but only
// once the log has grown past 2*retain lines — below that the file is left
// untouched, so a small log is a cheap no-op and rewrites amortize to at most
// once per retain appends. retain must exceed the recency window (callers derive
// it from the window with headroom): the reader only ever consults the last
// window entries, so retaining a strictly larger tail guarantees compaction can
// never change an eligibility result. A non-positive retain is a no-op, mirroring
// the reader's non-positive-window short-circuit.
//
// Compaction shares recent's validity rule: only lines that parse to an entry
// with a non-empty id are retained. Counting raw lines toward the tail could let
// a run of trailing junk evict valid entries the window still needs, so filtering
// by the reader's own rule keeps the invariant airtight and garbage-collects a
// corrupt or over-long line as a side effect rather than preserving it.
//
// The rewrite is crash-safe: the tail is written to a temp file in the same
// directory, synced, and atomically renamed over the log, so a crash leaves
// either the intact old log or the complete new one. It does not fsync the
// directory — matching Record's reliance on POSIX semantics; a lost rename just
// leaves the old log for the next run to retry. A concurrent session appending
// between the read and the rename has that append clobbered by the rename; this
// is accepted (see the package doc's ordering-by-append-time tradeoff) rather
// than guarded with a lock, since the cost is at most one item resurfacing a
// rotation early for a daemonless, single-session-per-invocation tool.
func (s *Store) Compact(_ context.Context, retain int) error {
	if retain <= 0 {
		return nil
	}

	f, err := os.Open(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("reading history %s: %w", s.path, err)
	}

	// tail holds the most recent retain valid lines (content without the trailing
	// newline), overwriting oldest-first once full; total counts every physical
	// line so the high-water decision reflects the true on-disk size, not the
	// retained size. The reader discipline matches recent: ReadString tolerates a
	// line over bufio.Scanner's 64K cap and a crash-truncated final line.
	tail := make([]string, 0, retain)
	next := 0
	total := 0
	reader := bufio.NewReader(f)
	var readErr error
	for {
		line, err := reader.ReadString('\n')
		if len(line) > 0 {
			total++
			content := strings.TrimSuffix(line, "\n")
			var e entry
			if jsonErr := json.Unmarshal([]byte(content), &e); jsonErr == nil && e.ID != "" {
				if len(tail) < retain {
					tail = append(tail, content)
				} else {
					tail[next] = content
					next = (next + 1) % retain
				}
			}
		}
		if err != nil {
			if !errors.Is(err, io.EOF) {
				readErr = err
			}
			break
		}
	}
	if closeErr := f.Close(); closeErr != nil && readErr == nil {
		readErr = closeErr
	}
	if readErr != nil {
		return fmt.Errorf("reading history %s: %w", s.path, readErr)
	}

	// Leave the log alone until it has grown well past the retained size: below
	// the high-water mark a rewrite would churn the file for little benefit.
	if total <= 2*retain {
		return nil
	}

	// Reassemble the ring oldest-first; once it wrapped, next points at the oldest
	// slot. When it never filled, next is 0 and this is the slice unchanged.
	ordered := make([]string, 0, len(tail))
	ordered = append(ordered, tail[next:]...)
	ordered = append(ordered, tail[:next]...)
	return s.rewrite(ordered)
}

// rewrite atomically replaces the log with lines, each newline-terminated, via a
// temp file in the same directory and a rename. The temp is synced before the
// rename so a crash cannot leave a renamed but partial file, and removed if any
// step before the rename fails so it does not linger.
func (s *Store) rewrite(lines []string) error {
	dir := filepath.Dir(s.path)
	tmp, err := os.CreateTemp(dir, "history-*.jsonl")
	if err != nil {
		return fmt.Errorf("creating temp history in %s: %w", dir, err)
	}
	// fail returns the real error joined with the temp cleanup result; errors.Join
	// drops a nil remove and, by using its result, satisfies the linter's
	// check-blank (a bare _ = os.Remove would be rejected).
	fail := func(e error) error { return errors.Join(e, os.Remove(tmp.Name())) }

	var buf []byte
	for _, line := range lines {
		buf = append(buf, line...)
		buf = append(buf, '\n')
	}

	_, writeErr := tmp.Write(buf)
	var syncErr error
	if writeErr == nil {
		syncErr = tmp.Sync()
	}
	// Close is checked explicitly rather than deferred: like Record, a Close error
	// on a write can be the first sign the bytes never reached disk.
	closeErr := tmp.Close()
	if writeErr != nil {
		return fail(fmt.Errorf("writing temp history %s: %w", tmp.Name(), writeErr))
	}
	if syncErr != nil {
		return fail(fmt.Errorf("syncing temp history %s: %w", tmp.Name(), syncErr))
	}
	if closeErr != nil {
		return fail(fmt.Errorf("closing temp history %s: %w", tmp.Name(), closeErr))
	}

	if err := os.Rename(tmp.Name(), s.path); err != nil {
		return fail(fmt.Errorf("replacing history %s: %w", s.path, err))
	}
	return nil
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
