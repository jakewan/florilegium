// Package corpus loads and validates the user-supplied anthology: a YAML file
// of tagged items with optional opaque metadata. It guards the failure modes
// that would silently corrupt selection later — duplicate or missing ids, empty
// text, malformed or empty YAML — turning each into a specific, actionable error
// at load time so a bad corpus fails up front rather than as wrong behavior
// downstream.
package corpus

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// SupportedFormatVersion is the corpus format version this build understands. It
// is bumped on a breaking change to the corpus shape; version 1 is the baseline
// the first tagged release anchors. A corpus may omit the version field to accept
// the baseline, or set it explicitly; a value naming any other version is
// rejected so a corpus authored against an incompatible format fails loudly
// rather than degrading into wrong selection behavior.
const SupportedFormatVersion = 1

// Item is one validated corpus entry. ID and Text are required; the history
// store keys on ID, so it must be present and unique. Tags and Meta are
// optional. Tags is the categorical, queryable axis (list_tags, tag filtering).
// Meta is an opaque key/value map the server carries and returns verbatim but
// never interprets or queries — callers assign meaning to keys like
// "attribution" or "source" by convention.
type Item struct {
	ID   string            `yaml:"id"`
	Text string            `yaml:"text"`
	Meta map[string]string `yaml:"meta"`
	Tags []string          `yaml:"tags"`
}

// Corpus is the validated, in-memory anthology, holding items in file order.
type Corpus struct {
	// Version is the corpus format version. It is optional: an absent (zero)
	// value means the baseline (SupportedFormatVersion). This is the top-level
	// format axis and is unrelated to any "version" key a caller may place inside
	// an item's opaque Meta map.
	Version int    `yaml:"version"`
	Items   []Item `yaml:"items"`
}

// Load reads, parses, and validates the corpus YAML at path. It takes a context
// for consistency with the config and history loaders on the same startup path,
// though corpus reads are not yet cancellable. On any failure it returns a nil
// Corpus and an actionable error naming the offending path, position, or id; an
// empty corpus is one such failure, not an empty success.
func Load(_ context.Context, path string) (*Corpus, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("corpus not found at %s: create it or fix the config's corpus path", path)
		}
		return nil, fmt.Errorf("reading corpus %s: %w", path, err)
	}

	// KnownFields rejects unknown keys so a stale or mistyped field fails loudly
	// instead of being silently dropped — the failure mode a versioned format
	// exists to prevent. A document with no nodes (an empty or comment-only file)
	// makes Decode return io.EOF; that is the empty-corpus case, checked before
	// the generic parse wrap so it surfaces as the actionable "empty" message
	// rather than an opaque parse error.
	var c Corpus
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err = dec.Decode(&c); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("parsing corpus %s: %w", path, err)
	}

	// Version incompatibility is reported ahead of emptiness: a corpus targeting a
	// format this build cannot read is wrong at a more fundamental level than
	// having no items, so name that first. A zero (absent) version means the
	// baseline.
	if c.Version != 0 && c.Version != SupportedFormatVersion {
		return nil, fmt.Errorf("corpus %s: unsupported format version %d (this build supports version %d); upgrade florilegium or migrate the corpus", path, c.Version, SupportedFormatVersion)
	}

	if len(c.Items) == 0 {
		return nil, fmt.Errorf("corpus %s is empty: it must define at least one item under \"items\"", path)
	}

	// seen guards against duplicate ids, which would make the history store's
	// per-id bookkeeping ambiguous. It is local to validation; no by-id index is
	// retained until a consumer needs one.
	seen := make(map[string]struct{}, len(c.Items))
	for i, it := range c.Items {
		// Position is 1-based and reported when there is no id to name the item by.
		pos := i + 1
		// Trim before the emptiness check so a whitespace-only id is rejected like
		// a missing one — a blank id cannot serve as the stable history key.
		if strings.TrimSpace(it.ID) == "" {
			return nil, fmt.Errorf("corpus %s: item %d is missing an \"id\"", path, pos)
		}
		if strings.TrimSpace(it.Text) == "" {
			return nil, fmt.Errorf("corpus %s: item %q has empty \"text\"", path, it.ID)
		}
		if _, dup := seen[it.ID]; dup {
			return nil, fmt.Errorf("corpus %s: duplicate id %q", path, it.ID)
		}
		seen[it.ID] = struct{}{}
	}

	return &c, nil
}
