// Package config loads and validates the florilegium server configuration:
// the corpus path and the recency window. It does not read the corpus itself —
// that is the corpus loader's job — so a config can be valid here while the
// corpus it points at is absent.
package config

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config is the validated server configuration.
type Config struct {
	// Corpus is the path to the corpus YAML file, with a leading ~ expanded.
	// The file is not opened during Load; existence is checked when the corpus
	// is actually read.
	Corpus string
	// RecencyWindow is the number of recent picks to exclude from candidates.
	RecencyWindow int
}

// file mirrors the on-disk YAML shape. It is unexported and separate from
// Config so the public type can stay flat (a scalar window) without coupling
// callers to the file's nesting.
type file struct {
	Corpus  string `yaml:"corpus"`
	Recency struct {
		Window int `yaml:"window"`
	} `yaml:"recency"`
}

// Load reads and validates the config from the XDG config directory. It takes
// a context for consistency with the corpus and history loaders that read
// I/O on the same startup path, though config reads are not yet cancellable.
// On any failure it returns a nil Config and an actionable error naming the
// offending path or field — startup callers surface this instead of panicking.
func Load(_ context.Context) (*Config, error) {
	path, err := configPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("config not found at %s: create it with a corpus path and recency.window", path)
		}
		return nil, fmt.Errorf("reading config %s: %w", path, err)
	}

	var f file
	if err = yaml.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("parsing config %s: %w", path, err)
	}

	if f.Corpus == "" {
		return nil, fmt.Errorf(`config %s: field "corpus" is required`, path)
	}
	// A missing recency.window and an explicit 0 both unmarshal to 0; both are
	// invalid, so one positive check covers absent, zero, and negative.
	if f.Recency.Window <= 0 {
		return nil, fmt.Errorf(`config %s: field "recency.window" must be a positive integer, got %d`, path, f.Recency.Window)
	}

	corpus, err := expandTilde(f.Corpus)
	if err != nil {
		return nil, err
	}

	return &Config{Corpus: corpus, RecencyWindow: f.Recency.Window}, nil
}

// configPath resolves $XDG_CONFIG_HOME/florilegium/config.yml, falling back to
// the XDG default of ~/.config when the variable is unset or empty. It reads
// the env var directly (rather than os.UserConfigDir) so the path matches the
// documented XDG layout on every platform and stays test-isolable via t.Setenv.
func configPath() (string, error) {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolving config dir: %w", err)
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "florilegium", "config.yml"), nil
}

// expandTilde rewrites a leading ~ or ~/ to the user's home directory. Paths
// without a leading ~ are returned unchanged.
func expandTilde(path string) (string, error) {
	if path != "~" && !strings.HasPrefix(path, "~/") {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("expanding ~ in corpus path: %w", err)
	}
	if path == "~" {
		return home, nil
	}
	return filepath.Join(home, path[len("~/"):]), nil
}
