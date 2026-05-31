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
	// History is the path to the history log, with a leading ~ expanded, or ""
	// when the config omits it. An empty History means "use the default
	// location"; resolving that default is the caller's job (the composition
	// root), so config stays ignorant of the history package the way it stays
	// ignorant of the corpus loader.
	History string
	// RecencyWindow is the number of recent picks to exclude from candidates.
	RecencyWindow int
}

// file mirrors the on-disk YAML shape. It is unexported and separate from
// Config so the public type can stay flat (a scalar window) without coupling
// callers to the file's nesting.
type file struct {
	Corpus  string `yaml:"corpus"`
	History string `yaml:"history"`
	Recency struct {
		Window int `yaml:"window"`
	} `yaml:"recency"`
}

// Load reads and validates the config, resolving its path from override (the
// --config flag value), then $FLORILEGIUM_CONFIG, then the XDG config dir — so
// an instance can be pointed at its own config by a single florilegium-specific
// knob rather than by relocating the general-purpose XDG base. It takes a
// context for consistency with the corpus and history loaders that read I/O on
// the same startup path, though config reads are not yet cancellable. On any
// failure it returns a nil Config and an actionable error naming the offending
// path or field — startup callers surface this instead of panicking.
func Load(_ context.Context, override string) (*Config, error) {
	path, err := configPath(override)
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

	corpus, err := expandTilde(f.Corpus, "corpus path")
	if err != nil {
		return nil, err
	}

	// A blank history path is treated as absent: the field is optional, and an
	// empty or whitespace-only value carries no isolation intent, so it resolves
	// to "" and the caller applies the default location.
	var history string
	if strings.TrimSpace(f.History) != "" {
		history, err = expandTilde(f.History, "history path")
		if err != nil {
			return nil, err
		}
	}

	return &Config{Corpus: corpus, History: history, RecencyWindow: f.Recency.Window}, nil
}

// configPath resolves the config file path: the trimmed override (the --config
// flag value) if non-empty, then a trimmed $FLORILEGIUM_CONFIG, then
// $XDG_CONFIG_HOME/florilegium/config.yml (falling back to ~/.config when that
// is unset or empty). The first two point at the config file directly and are
// tilde-expanded; this florilegium-specific precedence lets one instance be
// isolated by a single knob rather than by relocating the general-purpose XDG
// base. Whitespace-only override or env values are ignored so a stray value
// cannot resolve to a nonsense path. The XDG var is read directly (rather than
// os.UserConfigDir) so the path matches the documented layout on every platform
// and stays test-isolable via t.Setenv.
func configPath(override string) (string, error) {
	if p := strings.TrimSpace(override); p != "" {
		return expandTilde(p, "--config path")
	}
	if p := strings.TrimSpace(os.Getenv("FLORILEGIUM_CONFIG")); p != "" {
		return expandTilde(p, "FLORILEGIUM_CONFIG path")
	}
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
// without a leading ~ are returned unchanged. field names what is being
// expanded so an expansion failure points at the offending input.
func expandTilde(path, field string) (string, error) {
	if path != "~" && !strings.HasPrefix(path, "~/") {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("expanding ~ in %s: %w", field, err)
	}
	if path == "~" {
		return home, nil
	}
	return filepath.Join(home, path[len("~/"):]), nil
}
