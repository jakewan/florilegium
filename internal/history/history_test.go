package history

import (
	"context"
	"os"
	"path/filepath"
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
