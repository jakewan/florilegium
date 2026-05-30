# florilegium

A general-purpose [MCP](https://modelcontextprotocol.io) server that surfaces one apt item at a time from a user-supplied corpus — recency-aware, without recent repeats — and leaves the question of *which* item fits to the calling agent.

> A *florilegium* (Latin *flos* "flower" + *legere* "to gather") is a curated anthology of choice extracts gathered from many sources. The name is the data model: you bring the anthology; the server gathers from it.

> **Status: early.** The repository is scaffolded — it builds, tests, lints, and installs a runnable MCP server binary that loads its config at startup and registers its tool contract for discovery, but the tool handlers are stubbed. The corpus, history store, and handler logic are tracked in the issues; see them for the planned build order.

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

These describe the intended contract — none are implemented yet; see the issues for build order.

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

The binary builds and runs today — it requires a config file at startup and exposes its tools for discovery — but the handlers are stubbed, so useful end-to-end behavior arrives with the corpus, history, and selection work tracked in the issues.

## License

Released under the [MIT License](LICENSE).
