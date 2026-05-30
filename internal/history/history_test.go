package history

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestEligible describes recency filtering from the caller's perspective: given
// a sequence of recorded uses and a count-based window, which candidate ids are
// still eligible. Each case records its own uses into a fresh store under
// t.TempDir(), so filesystem state is isolated.
func TestEligible(t *testing.T) {
	tests := []struct {
		name string
		// record is the ordered sequence of ids surfaced (oldest first).
		record     []string
		candidates []string
		window     int
		want       []string
	}{
		{
			name:       "no history makes every candidate eligible",
			record:     nil,
			candidates: []string{"a", "b", "c"},
			window:     2,
			want:       []string{"a", "b", "c"},
		},
		{
			name:       "a use within the window is excluded",
			record:     []string{"a"},
			candidates: []string{"a", "b", "c"},
			window:     2,
			want:       []string{"b", "c"},
		},
		{
			name:       "a use pushed outside the window is eligible again",
			record:     []string{"a", "b", "c"},
			candidates: []string{"a", "b", "c"},
			window:     2, // last two picks are b, c — a has aged out
			want:       []string{"a"},
		},
		{
			name:       "a window wider than the history excludes the whole history",
			record:     []string{"a"},
			candidates: []string{"a", "b", "c"},
			window:     5,
			want:       []string{"b", "c"},
		},
		{
			name:       "a re-used id is excluded even if its first use aged out",
			record:     []string{"a", "b", "a"},
			candidates: []string{"a", "b", "c"},
			window:     2, // last two picks are b, a
			want:       []string{"c"},
		},
		{
			name:       "candidate order is preserved",
			record:     []string{"b"},
			candidates: []string{"c", "b", "a"},
			window:     2,
			want:       []string{"c", "a"},
		},
		{
			name:       "a non-positive window excludes nothing",
			record:     []string{"a", "b"},
			candidates: []string{"a", "b", "c"},
			window:     0,
			want:       []string{"a", "b", "c"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			s := New(filepath.Join(t.TempDir(), "florilegium", "history.jsonl"))
			for _, id := range tt.record {
				if err := s.Record(ctx, id); err != nil {
					t.Fatalf("Record(%q): %v", id, err)
				}
			}

			got, err := s.Eligible(ctx, tt.candidates, tt.window)
			if err != nil {
				t.Fatalf("Eligible: %v", err)
			}
			if !equal(got, tt.want) {
				t.Errorf("Eligible = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestRecordPersistsAcrossStores covers the durability requirement: a use
// recorded through one store is visible to a separately constructed store at the
// same path, so recency survives the process exiting between sessions.
func TestRecordPersistsAcrossStores(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "florilegium", "history.jsonl")

	if err := New(path).Record(ctx, "a"); err != nil {
		t.Fatalf("Record: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("history file not created: %v", err)
	}

	got, err := New(path).Eligible(ctx, []string{"a", "b"}, 1)
	if err != nil {
		t.Fatalf("Eligible: %v", err)
	}
	if want := []string{"b"}; !equal(got, want) {
		t.Errorf("Eligible = %v, want %v (recorded use should persist)", got, want)
	}
}

// TestEligibleSkipsOverlongLine covers reader robustness: a line longer than
// bufio.Scanner's 64K cap must not fail the whole read. The over-long line is
// skipped and a valid entry after it still counts toward recency.
func TestEligibleSkipsOverlongLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "florilegium", "history.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("setup: %v", err)
	}
	// A 128K junk line exceeds bufio.Scanner's default token cap; a valid entry
	// follows it and must still be read.
	content := strings.Repeat("x", 128*1024) + "\n" +
		`{"id":"a","at":"2026-05-30T00:00:00Z"}` + "\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}

	got, err := New(path).Eligible(context.Background(), []string{"a", "b"}, 1)
	if err != nil {
		t.Fatalf("Eligible: %v", err)
	}
	if want := []string{"b"}; !equal(got, want) {
		t.Errorf("Eligible = %v, want %v (overlong line skipped, valid entry counted)", got, want)
	}
}

// TestRecordRejectsEmptyID covers that Record refuses an empty or
// whitespace-only id rather than persisting an entry the reader would skip,
// which would be a silent no-op that still grows the log and hides the caller
// bug. No file should be written on rejection.
func TestRecordRejectsEmptyID(t *testing.T) {
	for _, id := range []string{"", "   "} {
		t.Run("id="+id, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "florilegium", "history.jsonl")
			if err := New(path).Record(context.Background(), id); err == nil {
				t.Fatalf("Record(%q) = nil, want error", id)
			}
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Errorf("history file exists after rejected Record, want absent")
			}
		})
	}
}

// TestDefaultPath checks that the default history location follows the XDG state
// layout, honoring XDG_STATE_HOME and falling back to ~/.local/state, so the
// path matches the documented layout and stays test-isolable via t.Setenv.
func TestDefaultPath(t *testing.T) {
	t.Run("honors XDG_STATE_HOME", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("XDG_STATE_HOME", dir)

		got, err := DefaultPath()
		if err != nil {
			t.Fatalf("DefaultPath: %v", err)
		}
		if want := filepath.Join(dir, "florilegium", "history.jsonl"); got != want {
			t.Errorf("DefaultPath = %q, want %q", got, want)
		}
	})

	t.Run("falls back to ~/.local/state when unset", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("XDG_STATE_HOME", "")
		t.Setenv("HOME", dir)

		got, err := DefaultPath()
		if err != nil {
			t.Fatalf("DefaultPath: %v", err)
		}
		if want := filepath.Join(dir, ".local", "state", "florilegium", "history.jsonl"); got != want {
			t.Errorf("DefaultPath = %q, want %q", got, want)
		}
	})
}

// TestCompactMissingFileIsNoOp covers that compacting a log that was never
// written is a silent no-op: a first run before any use should neither error
// nor materialize an empty file.
func TestCompactMissingFileIsNoOp(t *testing.T) {
	path := filepath.Join(t.TempDir(), "florilegium", "history.jsonl")
	if err := New(path).Compact(context.Background(), 5); err != nil {
		t.Fatalf("Compact on missing file: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("history file exists after Compact on missing log, want absent")
	}
}

// TestCompactNonPositiveRetainIsNoOp covers the retain<=0 guard, mirroring the
// reader's window<=0 short-circuit: there is no meaningful tail to keep, so the
// log is left untouched rather than truncated to nothing.
func TestCompactNonPositiveRetainIsNoOp(t *testing.T) {
	path := filepath.Join(t.TempDir(), "florilegium", "history.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("setup: %v", err)
	}
	content := `{"id":"a","at":"2026-05-30T00:00:00Z"}` + "\n" +
		`{"id":"b","at":"2026-05-30T00:00:01Z"}` + "\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}

	if err := New(path).Compact(context.Background(), 0); err != nil {
		t.Fatalf("Compact(retain=0): %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading history: %v", err)
	}
	if string(got) != content {
		t.Errorf("Compact(retain=0) modified the log:\n got %q\nwant %q", got, content)
	}
}

// TestCompactSmallLogIsNoOp covers the high-water-mark guard: a log at or below
// 2*retain lines is not worth rewriting, so it must be left byte-for-byte
// unchanged. Exactly 2*retain lines is the boundary that must still no-op.
func TestCompactSmallLogIsNoOp(t *testing.T) {
	const retain = 5
	path := filepath.Join(t.TempDir(), "florilegium", "history.jsonl")
	s := New(path)
	recordIDs(t, s, seqIDs(2*retain)) // exactly the threshold — must not rewrite

	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading history: %v", err)
	}

	if err = s.Compact(context.Background(), retain); err != nil {
		t.Fatalf("Compact: %v", err)
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading history: %v", err)
	}
	if string(before) != string(after) {
		t.Errorf("Compact rewrote a log at the threshold; want it untouched")
	}
}

// TestCompactPreservesEligibility is the load-bearing invariant: trimming the
// log must never change which candidates are eligible for any window the store
// can be asked about. Recording past the high-water mark forces a real rewrite,
// then eligibility is compared across several windows no wider than retain.
func TestCompactPreservesEligibility(t *testing.T) {
	const retain = 5
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "florilegium", "history.jsonl")
	s := New(path)
	recordIDs(t, s, seqIDs(30)) // 30 > 2*retain, so Compact will rewrite

	candidates := []string{"id00", "id24", "id26", "id29", "missing"}
	windows := []int{1, 3, retain}
	before := make(map[int][]string, len(windows))
	for _, w := range windows {
		got, err := s.Eligible(ctx, candidates, w)
		if err != nil {
			t.Fatalf("Eligible(window=%d): %v", w, err)
		}
		before[w] = got
	}

	if err := s.Compact(ctx, retain); err != nil {
		t.Fatalf("Compact: %v", err)
	}

	for _, w := range windows {
		got, err := s.Eligible(ctx, candidates, w)
		if err != nil {
			t.Fatalf("Eligible(window=%d) after compaction: %v", w, err)
		}
		if !equal(got, before[w]) {
			t.Errorf("window=%d: Eligible after compaction = %v, want %v (must be unchanged)", w, got, before[w])
		}
	}
}

// TestCompactTrimsToRetainTail covers that a rewrite keeps exactly the most
// recent retain entries, in order — the tail the reader would consult — and
// drops everything older.
func TestCompactTrimsToRetainTail(t *testing.T) {
	const retain = 5
	path := filepath.Join(t.TempDir(), "florilegium", "history.jsonl")
	s := New(path)
	recordIDs(t, s, seqIDs(30))

	full := readHistoryLines(t, path)
	wantTail := full[len(full)-retain:]

	if err := s.Compact(context.Background(), retain); err != nil {
		t.Fatalf("Compact: %v", err)
	}

	if got := readHistoryLines(t, path); !equal(got, wantTail) {
		t.Errorf("trimmed log = %v, want last %d lines %v", got, retain, wantTail)
	}
}

// TestCompactThenRecord covers that the rewritten log is still a well-formed,
// appendable JSON-lines file: a use recorded after compaction lands on its own
// line and is visible to the reader, so the trailing-newline handling is right.
func TestCompactThenRecord(t *testing.T) {
	const retain = 5
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "florilegium", "history.jsonl")
	s := New(path)
	recordIDs(t, s, seqIDs(30))
	if err := s.Compact(ctx, retain); err != nil {
		t.Fatalf("Compact: %v", err)
	}

	if err := s.Record(ctx, "fresh"); err != nil {
		t.Fatalf("Record after compaction: %v", err)
	}

	// "fresh" is the most recent pick, so it is excluded at window 1 while a
	// retained-but-older id stays eligible.
	got, err := s.Eligible(ctx, []string{"fresh", "id00"}, 1)
	if err != nil {
		t.Fatalf("Eligible: %v", err)
	}
	if want := []string{"id00"}; !equal(got, want) {
		t.Errorf("Eligible after compact+record = %v, want %v", got, want)
	}
	if lines := readHistoryLines(t, path); len(lines) != retain+1 {
		t.Errorf("log has %d lines after compact+record, want %d (clean append)", len(lines), retain+1)
	}
}

// TestCompactLeavesNoTempFiles covers that the temp file used for the atomic
// rewrite never lingers on success — the directory holds only the log.
func TestCompactLeavesNoTempFiles(t *testing.T) {
	const retain = 5
	dir := filepath.Join(t.TempDir(), "florilegium")
	path := filepath.Join(dir, "history.jsonl")
	s := New(path)
	recordIDs(t, s, seqIDs(30))

	if err := s.Compact(context.Background(), retain); err != nil {
		t.Fatalf("Compact: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading dir: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "history.jsonl" {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("dir contains %v, want only history.jsonl", names)
	}
}

// TestCompactToleratesOverlongLine covers that compaction shares the reader's
// validity rule: an over-long, unparseable line is skipped (not retained), the
// valid entries around it survive, and eligibility is unchanged. This both
// proves ReadString (not Scanner) is used and that corruption is garbage
// collected rather than counted toward the retained tail.
func TestCompactToleratesOverlongLine(t *testing.T) {
	const retain = 5
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "florilegium", "history.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("setup: %v", err)
	}
	// 8 valid entries, a 128K junk line, then 4 more valid: 13 physical lines
	// exceed 2*retain, so compaction rewrites; the junk line must not survive.
	var b strings.Builder
	for i := 0; i < 8; i++ {
		fmt.Fprintf(&b, `{"id":"id%02d","at":"2026-05-30T00:00:00Z"}`+"\n", i)
	}
	b.WriteString(strings.Repeat("x", 128*1024) + "\n")
	for i := 8; i < 12; i++ {
		fmt.Fprintf(&b, `{"id":"id%02d","at":"2026-05-30T00:00:00Z"}`+"\n", i)
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}

	s := New(path)
	candidates := []string{"id00", "id07", "id11"}
	beforeEligible, err := s.Eligible(ctx, candidates, retain)
	if err != nil {
		t.Fatalf("Eligible before compaction: %v", err)
	}

	if err = s.Compact(ctx, retain); err != nil {
		t.Fatalf("Compact: %v", err)
	}

	afterEligible, err := s.Eligible(ctx, candidates, retain)
	if err != nil {
		t.Fatalf("Eligible after compaction: %v", err)
	}
	if !equal(beforeEligible, afterEligible) {
		t.Errorf("eligibility changed across compaction: before %v, after %v", beforeEligible, afterEligible)
	}

	lines := readHistoryLines(t, path)
	if len(lines) != retain {
		t.Fatalf("trimmed log has %d lines, want %d valid entries", len(lines), retain)
	}
	for _, ln := range lines {
		if len(ln) > 1024 {
			t.Errorf("an over-long junk line survived compaction (len=%d)", len(ln))
		}
	}
}

// recordIDs records each id in order through s, failing the test on any error.
func recordIDs(t *testing.T, s *Store, ids []string) {
	t.Helper()
	ctx := context.Background()
	for _, id := range ids {
		if err := s.Record(ctx, id); err != nil {
			t.Fatalf("Record(%q): %v", id, err)
		}
	}
}

// seqIDs returns n sequential ids ("id00", "id01", …) for seeding a log with
// distinct, order-revealing entries.
func seqIDs(n int) []string {
	ids := make([]string, n)
	for i := range ids {
		ids[i] = fmt.Sprintf("id%02d", i)
	}
	return ids
}

// readHistoryLines returns the non-empty lines of the history file at path, so a
// test can assert on the log's exact on-disk contents.
func readHistoryLines(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading history %s: %v", path, err)
	}
	var lines []string
	for _, ln := range strings.Split(string(data), "\n") {
		if ln != "" {
			lines = append(lines, ln)
		}
	}
	return lines
}

// equal reports whether two id slices match element-for-element, treating nil
// and empty as equal so order-preserving filter results compare cleanly.
func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
