# florilegium

A general-purpose [MCP](https://modelcontextprotocol.io) server that surfaces one apt item at a time from a user-supplied corpus — recency-aware, without recent repeats — and leaves the question of *which* item fits to the calling agent.

> A *florilegium* (Latin *flos* "flower" + *legere* "to gather") is a curated anthology of choice extracts gathered from many sources. The name is the data model: you bring the anthology; the server gathers from it.

> **Status: early but working.** The server builds, installs, and runs end-to-end: it loads a corpus and config at startup, serves candidates over MCP, records uses, and excludes recently-surfaced items. See [Getting started](#getting-started) for a minimal run. Compaction of the history log and other refinements are tracked in the issues.

## The idea

Picking a relevant quote, snippet, or passage is two jobs that want different owners:

- **Mechanism** — hold the corpus, remember what was used recently, serve candidates, record picks. Deterministic, stateful, boring. A good fit for a small program.
- **Judgment** — decide which candidate actually *fits* the moment. Semantic, contextual. A good fit for the LLM that has the context.

florilegium does only the first job. It never decides relevance — it hands the agent a shortlist (optionally narrowed by tag), excluding anything used recently, and lets the agent choose. The server has no idea what the items are *for*; that framing lives entirely in the caller.

This split is what makes it reusable. The first application is opening a code review with a fitting epigraph, but the same primitive serves daily-quote widgets, flashcard rotation, prompt-snippet libraries — anything shaped like "surface a fitting one from many, without repeating myself."

## How it works

A single binary speaking MCP over stdio. No daemon, no network service, no background process — it loads the corpus, reads and writes a small local history store, and exits when the session ends.

- **Corpus** — a user-supplied YAML file of tagged, attributed items.
- **History** — a local store (under `$XDG_STATE_HOME/florilegium/`) tracking when each item was last surfaced, so recent picks can be excluded.
- **Config** — a YAML file (under `$XDG_CONFIG_HOME/florilegium/`) pointing at the corpus and setting the recency window.

## MCP tools

| Tool | Purpose |
| --- | --- |
| `list_candidates(tags?, limit?, exclude_recent?)` | Return a shortlist of items (id, text, attribution, tags), excluding recently-used ones. The agent picks from these. |
| `record_use(id)` | Mark an item as used, so it drops out of rotation for the recency window. |
| `list_tags()` | List the tags present in the corpus, so the agent can narrow before choosing. |

## Corpus format

```yaml
items:
  - id: ggg-effective-mass        # stable id — history keys on this
    text: "Effective mass beats brute force. Land clean, not hard."
    attribution: "Gennady Golovkin"
    tags: [focus, precision]
  - id: shokunin-no-corners
    text: "The craftsman does not cut corners even where no one will look."
    attribution: "Shokunin tradition"
    tags: [dedication, integrity]
```

Stable `id`s are required — the history store keys on them, so renaming or removing an item is a deliberate act, not an accident of editing the text.

A ready-to-use [`example-corpus.yml`](example-corpus.yml) ships at the repo root; copy it as a starting point and replace the items with your own.

## Configuration

```yaml
# $XDG_CONFIG_HOME/florilegium/config.yml
corpus: ~/.config/florilegium/corpus.yml
recency:
  window: 30   # exclude items surfaced within the last N picks
```

## Installing

Following the sibling tooling pattern:

```sh
just install   # builds and installs the binary to ~/.local/bin
```

Ensure `~/.local/bin` is on your `PATH`.

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

**3. Register the server with your MCP client.** It speaks MCP over stdio and takes no arguments. With the Claude Code CLI:

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
