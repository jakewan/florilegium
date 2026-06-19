# Florilegium

A general-purpose [MCP](https://modelcontextprotocol.io) server that surfaces one apt item at a time from a user-supplied corpus — recency-aware, without recent repeats — and leaves the question of *which* item fits to the calling agent.

> A *florilegium* (Latin *flos* "flower" + *legere* "to gather") is a curated anthology of choice extracts gathered from many sources. The name is the data model: you bring the anthology; the server gathers from it.

> **Status: early but working.** The server builds, installs, and runs end-to-end: it loads a corpus and config at startup, serves candidates over MCP, records uses, and excludes recently-surfaced items. See [Getting started](#getting-started) for a minimal run. The history log is trimmed automatically at startup so it stays bounded; other refinements are tracked in the issues.

## The idea

Picking a relevant quote, snippet, or passage is two jobs that want different owners:

- **Mechanism** — hold the corpus, remember what was used recently, serve candidates, record picks. Deterministic, stateful, boring. A good fit for a small program.
- **Judgment** — decide which candidate actually *fits* the moment. Semantic, contextual. A good fit for the LLM that has the context.

Florilegium does only the first job. It never decides relevance — it hands the agent a shortlist (optionally narrowed by tag), excluding anything used recently, and lets the agent choose. The server has no idea what the items are *for*; that framing lives entirely in the caller.

This split is what makes it reusable. The first application is opening a code review with a fitting epigraph, but the same primitive serves daily-quote widgets, flashcard rotation, prompt-snippet libraries — anything shaped like "surface a fitting one from many, without repeating myself."

## How it works

A single binary speaking MCP over stdio. No daemon, no network service, no background process — it loads the corpus, reads and writes a small local history store, and exits when the session ends.

- **Corpus** — a user-supplied YAML file of tagged items with optional opaque metadata.
- **History** — a local store (by default under `$XDG_STATE_HOME/florilegium/`, or wherever the config's `history:` field points) tracking when each item was last surfaced, so recent picks can be excluded.
- **Config** — a YAML file (by default under `$XDG_CONFIG_HOME/florilegium/`, or wherever `--config` / `$FLORILEGIUM_CONFIG` points) naming the corpus, setting the recency window, and optionally relocating the history store.

## MCP tools

| Tool | Purpose |
| --- | --- |
| `list_candidates(tags?, limit?, exclude_recent?)` | Return a shortlist of items (id, text, meta, tags), excluding recently-used ones. The agent picks from these. |
| `record_use(id)` | Mark an item as used, so it drops out of rotation for the recency window. |
| `list_tags()` | List the tags present in the corpus, so the agent can narrow before choosing. |

## Corpus format

```yaml
items:
  - id: ggg-effective-mass        # stable id — history keys on this
    text: "Effective mass beats brute force. Land clean, not hard."
    meta:                         # opaque key/value map, carried verbatim
      attribution: "Gennady Golovkin"
      source: "Boxing interviews"
    tags: [focus, precision]
  - id: shokunin-no-corners
    text: "The craftsman does not cut corners even where no one will look."
    meta:
      attribution: "Shokunin tradition"
    tags: [dedication, integrity]
```

Stable `id`s are required — the history store keys on them, so renaming or removing an item is a deliberate act, not an accident of editing the text.

`tags` is the queryable axis (`list_tags`, tag filtering). `meta` is an opaque key/value map the server stores and returns verbatim but never interprets or queries — assign meaning to keys like `attribution`, `source`, or `kind` by your own convention. Both are optional. `meta` values are strings, so quote anything that looks numeric or boolean (e.g. `year: "2017"`) to keep it from being coerced.

A ready-to-use [`example-corpus.yml`](example-corpus.yml) ships at the repo root; copy it as a starting point and replace the items with your own.

## Configuration

```yaml
# $XDG_CONFIG_HOME/florilegium/config.yml
corpus: ~/.config/florilegium/corpus.yml
history: ~/.local/state/florilegium/history.jsonl   # optional; see below
recency:
  window: 30   # exclude items surfaced within the last N picks
```

`history:` is optional — omit it and the store defaults to `$XDG_STATE_HOME/florilegium/history.jsonl`. Set it to give an instance its own rotation state (and see [Running multiple corpora](#running-multiple-corpora)). Use an **absolute path** (or a `~/...` path, which is expanded): an MCP client launches the server with an undefined working directory, so a relative path resolves unpredictably — and a relative `history:` would silently create a tree under whatever directory the client happened to choose.

By default the config is read from `$XDG_CONFIG_HOME/florilegium/config.yml`. Two Florilegium-specific knobs override that, in precedence order:

1. `--config <path>` — a command-line flag.
2. `$FLORILEGIUM_CONFIG` — an environment variable.

Both point at the config file directly (a `~/...` path is expanded; use an absolute path for the reasons above) and let an instance be isolated by one Florilegium-specific knob rather than by relocating the general-purpose `$XDG_CONFIG_HOME`.

### Running multiple corpora

Florilegium serves one corpus per process; run several instances to serve several corpora. Give each instance its own config and its own history, so their recency windows stay independent:

```jsonc
{
  "mcpServers": {
    "quotes": {
      "command": "florilegium",
      "env": { "FLORILEGIUM_CONFIG": "/path/to/quotes/config.yml" }
    },
    "flashcards": {
      "command": "florilegium",
      "env": { "FLORILEGIUM_CONFIG": "/path/to/flashcards/config.yml" }
    }
  }
}
```

Each config names its own `corpus:` and its own `history:`, so one knob per instance (`FLORILEGIUM_CONFIG`) fully isolates it. If two instances share a history log — for example, both omit `history:` and so fall back to the same default — they interleave into one recency window and can falsely exclude each other's items when ids collide, so point each at its own history.

## Installing

**Prebuilt binary** — download the archive for your platform from the [latest release](https://github.com/jakewan/florilegium/releases/latest), extract it, and put `florilegium` on your `PATH`. Each release ships cross-compiled binaries, a cosign-signed `checksums.txt`, and SLSA build provenance; exact verification commands accompany the release notes.

**With `go install`:**

```sh
go install github.com/jakewan/florilegium/cmd/florilegium@latest
```

**From source** (the sibling tooling pattern):

```sh
just install   # builds and installs the binary to ~/.local/bin
```

Ensure the install directory (`~/.local/bin`, or `$(go env GOPATH)/bin` for `go install`) is on your `PATH`.

## Getting started

A minimal end-to-end run against the example corpus.

**1. Install the binary** (above): `just install`.

**2. Write a config.** Create `$XDG_CONFIG_HOME/florilegium/config.yml` (defaults to `~/.config/florilegium/config.yml`) and point it at the example corpus:

```yaml
corpus: /absolute/path/to/florilegium/example-corpus.yml
recency:
  window: 3   # exclude items surfaced within the last N picks
```

Use an **absolute path** (or a `~/...` path, which is expanded) for `corpus:`. An MCP client launches the server with an undefined working directory, so a relative path will not resolve reliably.

**3. Register the server with your MCP client.** It speaks MCP over stdio and takes no arguments by default (an optional `--config` flag overrides the config path). With the Claude Code CLI:

```sh
claude mcp add florilegium -- florilegium
```

Or add it to a client's config directly:

```json
{
  "mcpServers": {
    "florilegium": {
      "command": "florilegium"
    }
  }
}
```

**4. Use it.** Once connected, the caller drives three tools:

- `list_tags()` → the tags present in the corpus (here: `calm`, `dedication`, `focus`, `integrity`, `precision`), so you can narrow before choosing.
- `list_candidates(tags?, limit?, exclude_recent?)` → a shortlist of items, with anything used within the recency window already excluded (`exclude_recent` defaults to `true`).
- `record_use(id)` → mark the chosen item as used.

The agent calls `list_candidates`, picks the one that fits, then calls `record_use` with its `id`. That id now drops out of `list_candidates` until `window` newer picks push it past the recency window. Uses are recorded to a history log under `$XDG_STATE_HOME/florilegium/`, so the rotation persists across sessions.

## License

Released under the [MIT License](LICENSE).
