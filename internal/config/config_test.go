package config

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLoad describes config loading from the caller's perspective: where the
// file is found, which malformed inputs fail with an actionable message, and
// what a valid file parses into. Filesystem state is isolated per case with
// t.TempDir() and the relevant env vars via t.Setenv.
func TestLoad(t *testing.T) {
	tests := []struct {
		name string
		// write controls whether a config file is created; false leaves the
		// config path absent to exercise the missing-file path.
		write   bool
		content string
		wantErr string // substring the error must contain; "" means expect success
		check   func(t *testing.T, cfg *Config, home string)
	}{
		{
			name:    "missing file",
			write:   false,
			wantErr: "config.yml", // message must name the resolved path
		},
		{
			name:    "malformed yaml",
			write:   true,
			content: "corpus: [unclosed",
			wantErr: "parsing config",
		},
		{
			name:    "missing corpus field",
			write:   true,
			content: "recency:\n  window: 30\n",
			wantErr: `"corpus"`,
		},
		{
			name:    "missing window",
			write:   true,
			content: "corpus: ~/corpus.yml\n",
			wantErr: `"recency.window"`,
		},
		{
			name:    "zero window",
			write:   true,
			content: "corpus: ~/corpus.yml\nrecency:\n  window: 0\n",
			wantErr: "positive",
		},
		{
			name:    "negative window",
			write:   true,
			content: "corpus: ~/corpus.yml\nrecency:\n  window: -5\n",
			wantErr: "-5", // message names the offending value
		},
		{
			name:    "valid with tilde expansion",
			write:   true,
			content: "corpus: ~/corpus.yml\nrecency:\n  window: 30\n",
			check: func(t *testing.T, cfg *Config, home string) {
				if cfg.RecencyWindow != 30 {
					t.Errorf("RecencyWindow = %d, want 30", cfg.RecencyWindow)
				}
				want := filepath.Join(home, "corpus.yml")
				if cfg.Corpus != want {
					t.Errorf("Corpus = %q, want %q (tilde expanded)", cfg.Corpus, want)
				}
				if strings.Contains(cfg.Corpus, "~") {
					t.Errorf("Corpus %q still contains a literal ~", cfg.Corpus)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			xdg := t.TempDir()
			home := t.TempDir()
			t.Setenv("XDG_CONFIG_HOME", xdg)
			t.Setenv("HOME", home)

			if tt.write {
				dir := filepath.Join(xdg, "florilegium")
				if err := os.MkdirAll(dir, 0o755); err != nil {
					t.Fatalf("setup: %v", err)
				}
				path := filepath.Join(dir, "config.yml")
				if err := os.WriteFile(path, []byte(tt.content), 0o644); err != nil {
					t.Fatalf("setup: %v", err)
				}
			}

			cfg, err := Load(context.Background())

			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("Load() error = nil, want error containing %q", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("Load() error = %q, want substring %q", err, tt.wantErr)
				}
				if cfg != nil {
					t.Errorf("Load() cfg = %+v, want nil on error", cfg)
				}
				return
			}

			if err != nil {
				t.Fatalf("Load() unexpected error: %v", err)
			}
			if cfg == nil {
				t.Fatal("Load() cfg = nil, want non-nil")
			}
			if tt.check != nil {
				tt.check(t, cfg, home)
			}
		})
	}
}
