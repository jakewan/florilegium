// Package corpus loads and validates the user-supplied anthology: a YAML file
// of tagged items with optional opaque metadata. It guards the failure modes
// that would silently corrupt selection later — duplicate or missing ids, empty
// text, malformed or empty YAML — turning each into a specific, actionable error
// at load time so a bad corpus fails up front rather than as wrong behavior
// downstream.
package corpus

import (
	"context"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

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
	Items []Item `yaml:"items"`
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

	var c Corpus
	if err = yaml.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parsing corpus %s: %w", path, err)
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
