package config

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLoad describes config loading from the caller's perspective: how the
// config path is resolved (the --config override, the FLORILEGIUM_CONFIG env
// var, then the XDG default), which malformed inputs fail with an actionable
// message, and what a valid file parses into — including the optional history
// path. Filesystem state is isolated per case with t.TempDir() and the relevant
// env vars via t.Setenv.
//
// Each case's setup writes whatever config files it needs and returns the
// override (the --config flag value) and env (FLORILEGIUM_CONFIG) for that
// case; the runner sets HOME, XDG_CONFIG_HOME, and FLORILEGIUM_CONFIG before
// calling Load. FLORILEGIUM_CONFIG is always set (to "" by default) so an
// ambient value cannot leak into a case.
func TestLoad(t *testing.T) {
	// writeConfig writes content to path, creating parent dirs.
	writeConfig := func(t *testing.T, path, content string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("setup: %v", err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}
	}
	// xdgConfig is the file Load resolves from the XDG default location.
	xdgConfig := func(xdg string) string {
		return filepath.Join(xdg, "florilegium", "config.yml")
	}

	const validWindow30 = "corpus: ~/corpus.yml\nrecency:\n  window: 30\n"

	tests := []struct {
		name    string
		setup   func(t *testing.T, xdg, home string) (override, env string)
		wantErr string // substring the error must contain; "" means expect success
		check   func(t *testing.T, cfg *Config, home string)
	}{
		{
			name: "missing file at XDG default",
			setup: func(t *testing.T, xdg, home string) (string, string) {
				return "", "" // write nothing
			},
			wantErr: "config.yml", // message must name the resolved path
		},
		{
			name: "malformed yaml",
			setup: func(t *testing.T, xdg, home string) (string, string) {
				writeConfig(t, xdgConfig(xdg), "corpus: [unclosed")
				return "", ""
			},
			wantErr: "parsing config",
		},
		{
			name: "missing corpus field",
			setup: func(t *testing.T, xdg, home string) (string, string) {
				writeConfig(t, xdgConfig(xdg), "recency:\n  window: 30\n")
				return "", ""
			},
			wantErr: `"corpus"`,
		},
		{
			name: "missing window",
			setup: func(t *testing.T, xdg, home string) (string, string) {
				writeConfig(t, xdgConfig(xdg), "corpus: ~/corpus.yml\n")
				return "", ""
			},
			wantErr: `"recency.window"`,
		},
		{
			name: "zero window",
			setup: func(t *testing.T, xdg, home string) (string, string) {
				writeConfig(t, xdgConfig(xdg), "corpus: ~/corpus.yml\nrecency:\n  window: 0\n")
				return "", ""
			},
			wantErr: "positive",
		},
		{
			name: "negative window",
			setup: func(t *testing.T, xdg, home string) (string, string) {
				writeConfig(t, xdgConfig(xdg), "corpus: ~/corpus.yml\nrecency:\n  window: -5\n")
				return "", ""
			},
			wantErr: "-5", // message names the offending value
		},
		{
			name: "valid at XDG default with tilde expansion and no history",
			setup: func(t *testing.T, xdg, home string) (string, string) {
				writeConfig(t, xdgConfig(xdg), validWindow30)
				return "", ""
			},
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
				if cfg.History != "" {
					t.Errorf("History = %q, want empty when the field is absent", cfg.History)
				}
			},
		},
		{
			name: "flag override is read",
			setup: func(t *testing.T, xdg, home string) (string, string) {
				path := filepath.Join(t.TempDir(), "custom.yml")
				writeConfig(t, path, "corpus: ~/c.yml\nrecency:\n  window: 11\n")
				return path, ""
			},
			check: func(t *testing.T, cfg *Config, home string) {
				if cfg.RecencyWindow != 11 {
					t.Errorf("RecencyWindow = %d, want 11 (from --config override)", cfg.RecencyWindow)
				}
			},
		},
		{
			name: "env var is read when no flag",
			setup: func(t *testing.T, xdg, home string) (string, string) {
				path := filepath.Join(t.TempDir(), "env.yml")
				writeConfig(t, path, "corpus: ~/c.yml\nrecency:\n  window: 22\n")
				return "", path
			},
			check: func(t *testing.T, cfg *Config, home string) {
				if cfg.RecencyWindow != 22 {
					t.Errorf("RecencyWindow = %d, want 22 (from FLORILEGIUM_CONFIG)", cfg.RecencyWindow)
				}
			},
		},
		{
			name: "flag wins over env",
			setup: func(t *testing.T, xdg, home string) (string, string) {
				dir := t.TempDir()
				ov := filepath.Join(dir, "flag.yml")
				env := filepath.Join(dir, "env.yml")
				writeConfig(t, ov, "corpus: ~/c.yml\nrecency:\n  window: 11\n")
				writeConfig(t, env, "corpus: ~/c.yml\nrecency:\n  window: 22\n")
				return ov, env
			},
			check: func(t *testing.T, cfg *Config, home string) {
				if cfg.RecencyWindow != 11 {
					t.Errorf("RecencyWindow = %d, want 11 (flag wins over env)", cfg.RecencyWindow)
				}
			},
		},
		{
			name: "whitespace flag and env fall through to XDG default",
			setup: func(t *testing.T, xdg, home string) (string, string) {
				writeConfig(t, xdgConfig(xdg), validWindow30)
				return "   ", "  "
			},
			check: func(t *testing.T, cfg *Config, home string) {
				if cfg.RecencyWindow != 30 {
					t.Errorf("RecencyWindow = %d, want 30 (whitespace overrides ignored)", cfg.RecencyWindow)
				}
			},
		},
		{
			name: "flag override with tilde expands",
			setup: func(t *testing.T, xdg, home string) (string, string) {
				writeConfig(t, filepath.Join(home, "sub", "config.yml"), "corpus: ~/c.yml\nrecency:\n  window: 7\n")
				return "~/sub/config.yml", ""
			},
			check: func(t *testing.T, cfg *Config, home string) {
				if cfg.RecencyWindow != 7 {
					t.Errorf("RecencyWindow = %d, want 7 (tilde override expanded)", cfg.RecencyWindow)
				}
			},
		},
		{
			name: "env override with tilde expands",
			setup: func(t *testing.T, xdg, home string) (string, string) {
				writeConfig(t, filepath.Join(home, "envsub", "config.yml"), "corpus: ~/c.yml\nrecency:\n  window: 8\n")
				return "", "~/envsub/config.yml"
			},
			check: func(t *testing.T, cfg *Config, home string) {
				if cfg.RecencyWindow != 8 {
					t.Errorf("RecencyWindow = %d, want 8 (tilde env expanded)", cfg.RecencyWindow)
				}
			},
		},
		{
			name: "missing override file names the override path",
			setup: func(t *testing.T, xdg, home string) (string, string) {
				// A valid XDG-default config exists, proving the override path —
				// not the default — is what gets read and reported.
				writeConfig(t, xdgConfig(xdg), validWindow30)
				return filepath.Join(t.TempDir(), "ghost.yml"), ""
			},
			wantErr: "ghost.yml",
		},
		{
			name: "history field parsed and tilde-expanded",
			setup: func(t *testing.T, xdg, home string) (string, string) {
				writeConfig(t, xdgConfig(xdg), "corpus: ~/c.yml\nhistory: ~/h.jsonl\nrecency:\n  window: 5\n")
				return "", ""
			},
			check: func(t *testing.T, cfg *Config, home string) {
				want := filepath.Join(home, "h.jsonl")
				if cfg.History != want {
					t.Errorf("History = %q, want %q (tilde expanded)", cfg.History, want)
				}
				if strings.Contains(cfg.History, "~") {
					t.Errorf("History %q still contains a literal ~", cfg.History)
				}
			},
		},
		{
			name: "empty history field resolves to empty",
			setup: func(t *testing.T, xdg, home string) (string, string) {
				writeConfig(t, xdgConfig(xdg), "corpus: ~/c.yml\nhistory: \"\"\nrecency:\n  window: 5\n")
				return "", ""
			},
			check: func(t *testing.T, cfg *Config, home string) {
				if cfg.History != "" {
					t.Errorf("History = %q, want empty when the field is blank", cfg.History)
				}
			},
		},
		{
			name: "whitespace history field resolves to empty",
			setup: func(t *testing.T, xdg, home string) (string, string) {
				writeConfig(t, xdgConfig(xdg), "corpus: ~/c.yml\nhistory: \"   \"\nrecency:\n  window: 5\n")
				return "", ""
			},
			check: func(t *testing.T, cfg *Config, home string) {
				if cfg.History != "" {
					t.Errorf("History = %q, want empty when the field is whitespace", cfg.History)
				}
			},
		},
		{
			name: "absolute history field round-trips unchanged",
			setup: func(t *testing.T, xdg, home string) (string, string) {
				writeConfig(t, xdgConfig(xdg), "corpus: ~/c.yml\nhistory: /var/lib/florilegium/h.jsonl\nrecency:\n  window: 5\n")
				return "", ""
			},
			check: func(t *testing.T, cfg *Config, home string) {
				want := "/var/lib/florilegium/h.jsonl"
				if cfg.History != want {
					t.Errorf("History = %q, want %q (absolute path unchanged)", cfg.History, want)
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

			override, env := tt.setup(t, xdg, home)
			t.Setenv("FLORILEGIUM_CONFIG", env)

			cfg, err := Load(context.Background(), override)

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
