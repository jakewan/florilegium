package corpus

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLoad describes corpus loading from the caller's perspective: what a
// well-formed file parses into, and which malformed inputs fail with an
// actionable message that names the offending id, position, or path. Each case
// writes its fixture into t.TempDir() so the filesystem state is isolated.
func TestLoad(t *testing.T) {
	tests := []struct {
		name string
		// write controls whether a corpus file is created; false leaves the path
		// absent to exercise the missing-file case.
		write   bool
		content string
		wantErr string // substring the error must contain; "" means expect success
		check   func(t *testing.T, c *Corpus)
	}{
		{
			name:    "missing file",
			write:   false,
			wantErr: "corpus.yml", // message must name the resolved path
		},
		{
			name:    "malformed yaml",
			write:   true,
			content: "items: [unclosed",
			wantErr: "parsing corpus",
		},
		{
			name:    "absent items key",
			write:   true,
			content: "# no items here\n",
			wantErr: "empty",
		},
		{
			name:    "empty items list",
			write:   true,
			content: "items: []\n",
			wantErr: "empty",
		},
		{
			name:  "missing id",
			write: true,
			content: "" +
				"items:\n" +
				"  - text: a thought\n",
			wantErr: "item 1", // names the offending item by 1-based position
		},
		{
			name:  "whitespace-only id",
			write: true,
			content: "" +
				"items:\n" +
				"  - id: \"   \"\n" +
				"    text: a thought\n",
			wantErr: "item 1", // a blank id is treated like a missing one — named by position
		},
		{
			name:  "missing text",
			write: true,
			content: "" +
				"items:\n" +
				"  - id: no-text\n",
			wantErr: "no-text", // names the offending id
		},
		{
			name:  "whitespace-only text",
			write: true,
			content: "" +
				"items:\n" +
				"  - id: blank-text\n" +
				"    text: \"   \"\n",
			wantErr: "blank-text",
		},
		{
			name:  "duplicate id",
			write: true,
			content: "" +
				"items:\n" +
				"  - id: dup\n" +
				"    text: first\n" +
				"  - id: dup\n" +
				"    text: second\n",
			wantErr: "dup",
		},
		{
			name:  "valid full item",
			write: true,
			content: "" +
				"items:\n" +
				"  - id: ggg-effective-mass\n" +
				"    text: \"Effective mass beats brute force.\"\n" +
				"    meta:\n" +
				"      attribution: \"Gennady Golovkin\"\n" +
				"      source: \"Boxing wisdom\"\n" +
				"    tags: [focus, precision]\n",
			check: func(t *testing.T, c *Corpus) {
				if len(c.Items) != 1 {
					t.Fatalf("len(Items) = %d, want 1", len(c.Items))
				}
				it := c.Items[0]
				if it.ID != "ggg-effective-mass" {
					t.Errorf("ID = %q, want ggg-effective-mass", it.ID)
				}
				if it.Text != "Effective mass beats brute force." {
					t.Errorf("Text = %q", it.Text)
				}
				if it.Meta["attribution"] != "Gennady Golovkin" {
					t.Errorf(`Meta["attribution"] = %q, want Gennady Golovkin`, it.Meta["attribution"])
				}
				if it.Meta["source"] != "Boxing wisdom" {
					t.Errorf(`Meta["source"] = %q, want Boxing wisdom`, it.Meta["source"])
				}
				if len(it.Tags) != 2 || it.Tags[0] != "focus" || it.Tags[1] != "precision" {
					t.Errorf("Tags = %v, want [focus precision]", it.Tags)
				}
			},
		},
		{
			name:  "meta and tags optional",
			write: true,
			content: "" +
				"items:\n" +
				"  - id: bare\n" +
				"    text: just text\n",
			check: func(t *testing.T, c *Corpus) {
				if len(c.Items) != 1 {
					t.Fatalf("len(Items) = %d, want 1", len(c.Items))
				}
				it := c.Items[0]
				if len(it.Meta) != 0 {
					t.Errorf("Meta = %v, want none", it.Meta)
				}
				if len(it.Tags) != 0 {
					t.Errorf("Tags = %v, want none", it.Tags)
				}
			},
		},
		{
			// Guards the value-type decision (map[string]string): values that look
			// numeric or boolean must round-trip as the exact strings written, not
			// be coerced. Quoting in YAML is the documented convention for such
			// values; this pins that quoted values survive verbatim.
			name:  "meta carries values verbatim",
			write: true,
			content: "" +
				"items:\n" +
				"  - id: versioned\n" +
				"    text: a thought\n" +
				"    meta:\n" +
				"      version: \"1.20\"\n" +
				"      published: \"no\"\n",
			check: func(t *testing.T, c *Corpus) {
				it := c.Items[0]
				if it.Meta["version"] != "1.20" {
					t.Errorf(`Meta["version"] = %q, want "1.20"`, it.Meta["version"])
				}
				if it.Meta["published"] != "no" {
					t.Errorf(`Meta["published"] = %q, want "no"`, it.Meta["published"])
				}
			},
		},
		{
			// Guards the loud-failure promise: with KnownFields enabled, a corpus
			// still carrying the removed item-level attribution field is rejected
			// with an actionable error rather than loading with the stray field
			// silently dropped, so a stale corpus surfaces at load time instead of
			// degrading into wrong behavior. "not found" is the strict-field signal
			// (yaml.v3: "field attribution not found in type corpus.Item"); matching
			// it — rather than the shared "parsing corpus" prefix — distinguishes a
			// strict-field rejection from a plain syntax error.
			name:  "legacy attribution field is rejected",
			write: true,
			content: "" +
				"items:\n" +
				"  - id: legacy\n" +
				"    text: still loads\n" +
				"    attribution: \"Old Author\"\n",
			wantErr: "not found",
		},
		{
			name:  "preserves file order",
			write: true,
			content: "" +
				"items:\n" +
				"  - id: first\n" +
				"    text: one\n" +
				"  - id: second\n" +
				"    text: two\n" +
				"  - id: third\n" +
				"    text: three\n",
			check: func(t *testing.T, c *Corpus) {
				want := []string{"first", "second", "third"}
				if len(c.Items) != len(want) {
					t.Fatalf("len(Items) = %d, want %d", len(c.Items), len(want))
				}
				for i, id := range want {
					if c.Items[i].ID != id {
						t.Errorf("Items[%d].ID = %q, want %q", i, c.Items[i].ID, id)
					}
				}
			},
		},
		{
			// An explicit baseline version loads like any other corpus — this case
			// is also the guard that Version is a real, correctly-tagged struct
			// field: were it missing, KnownFields would reject "version" as unknown.
			name:  "explicit baseline version loads",
			write: true,
			content: "" +
				"version: 1\n" +
				"items:\n" +
				"  - id: only\n" +
				"    text: a thought\n",
			check: func(t *testing.T, c *Corpus) {
				if len(c.Items) != 1 || c.Items[0].ID != "only" {
					t.Fatalf("Items = %+v, want one item id=only", c.Items)
				}
			},
		},
		{
			// Absent version defaults to the baseline, so existing unversioned
			// corpora keep loading.
			name:  "absent version loads as baseline",
			write: true,
			content: "" +
				"items:\n" +
				"  - id: only\n" +
				"    text: a thought\n",
			check: func(t *testing.T, c *Corpus) {
				if len(c.Items) != 1 || c.Items[0].ID != "only" {
					t.Fatalf("Items = %+v, want one item id=only", c.Items)
				}
			},
		},
		{
			// A version the build does not support fails loudly. Matching the custom
			// message — not a generic field error — proves the version gate ran,
			// rather than a KnownFields rejection masquerading as version validation.
			name:  "unsupported version is rejected",
			write: true,
			content: "" +
				"version: 2\n" +
				"items:\n" +
				"  - id: only\n" +
				"    text: a thought\n",
			wantErr: "unsupported format version",
		},
		{
			// The top-level strict-field axis (the attribution case covers the
			// item-level axis): an otherwise-valid corpus with one stray top-level
			// key is rejected, so a typo or stale field fails loudly.
			name:  "stray top-level key is rejected",
			write: true,
			content: "" +
				"bogus: x\n" +
				"items:\n" +
				"  - id: only\n" +
				"    text: a thought\n",
			wantErr: "not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "corpus.yml")
			if tt.write {
				if err := os.WriteFile(path, []byte(tt.content), 0o644); err != nil {
					t.Fatalf("setup: %v", err)
				}
			}

			c, err := Load(context.Background(), path)

			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("Load() error = nil, want error containing %q", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("Load() error = %q, want substring %q", err, tt.wantErr)
				}
				if c != nil {
					t.Errorf("Load() corpus = %+v, want nil on error", c)
				}
				return
			}

			if err != nil {
				t.Fatalf("Load() unexpected error: %v", err)
			}
			if c == nil {
				t.Fatal("Load() corpus = nil, want non-nil")
			}
			if tt.check != nil {
				tt.check(t, c)
			}
		})
	}
}
