# florilegium — Agent Guide

This file orients an AI agent (or a new contributor) working in this repository. It is self-contained: everything needed to work here is described below or in the linked in-repo files.

## What florilegium is

A general-purpose [MCP](https://modelcontextprotocol.io) server that surfaces one apt item at a time from a user-supplied corpus — recency-aware, without recent repeats — and leaves the question of *which* item fits to the calling agent.

The design splits two jobs that want different owners:

- **Mechanism** (this server's job): hold the corpus, remember what was used recently, serve candidates, record picks. Deterministic, stateful.
- **Judgment** (the caller's job): decide which candidate actually fits the moment. The server applies no relevance logic of its own.

See `README.md` for the full design, the planned MCP tools, and the corpus/config formats.

## Status and layout

The repository is scaffolded and builds a runnable binary, but the server exposes no tools yet. Build order is tracked in the GitHub issues.

```
cmd/florilegium/      # binary entry point (constructs the MCP server, speaks stdio)
```

Corpus loading, the history store, and the MCP tool handlers arrive in their own changes and will add packages (likely under an internal path) as that code lands. Do not create those packages speculatively — add them when a change needs them.

## Build, test, lint

Tool versions are managed by [mise](https://mise.jdx.dev/) (`mise.toml`); tasks run through [just](https://github.com/casey/just) (`justfile`). One-time setup:

```sh
mise install     # install pinned Go, golangci-lint, just, lefthook
just hooks       # install git hooks (lefthook)
```

Everyday commands:

```sh
just build       # build the binary to bin/
just test        # go test ./...
just lint        # golangci-lint run ./...
just fmt         # gofmt -w .
just tidy        # go mod tidy
just verify      # go mod verify
just install     # build and install to ~/.local/bin
```

Formatting is enforced by golangci-lint's configured formatters (`gofmt`, `goimports`) — there is no separate format-check step. The `lefthook` hooks run formatting on commit and lint/test on push.

## Development approach

This project uses [BDD][bdd]-style/outside-in [TDD][tdd] for non-trivial code: write a failing behavior test from the caller's perspective first, let it drive the API, then implement the minimum to pass and refactor under the test's safety net. Tests use the standard `testing` package (no external frameworks), favor table-driven cases, and isolate filesystem/history state with `t.TempDir()`. Skip the ceremony for trivial work (typos, single-line fixes, documentation, these instruction files).

Go authoring conventions are in `.claude/rules/go-practices.md` (loaded when editing Go).

## Key design decisions

- **Single binary, daemonless.** It loads the corpus, reads/writes a small local history store, serves a session over stdio, and exits. No background process, no network service.
- **MCP over stdio is JSON-RPC.** stdout carries the protocol and nothing else — send diagnostics to stderr (`log`), never to stdout. Exiting on stdin EOF is normal shutdown.
- **History is a flat, append-only JSON-lines file** (`history.jsonl` under `$XDG_STATE_HOME/florilegium/`), one `{"id","at"}` object per recorded use. The model is an ordered log and the only query is "the last N picks," so a SQL engine and migrations would be weight without benefit for a daemonless, single-process-per-session tool. Recency is count-based — eligibility excludes ids appearing in the most recent N entries. The log is trimmed at startup to a tail safely larger than the recency window, so it stays bounded without ever changing an eligibility result. Known tradeoff: cross-process ordering is by append time, and a second session appending while another is mid-trim can have that append clobbered by the trim's atomic rewrite — at most one item resurfaces a rotation early, accepted rather than locked against for a daemonless tool.

## Conventions in this repo

- `.claude/rules/go-practices.md` — Go authoring conventions (path-conditioned to Go files).
- `.claude/rules/pr-conventions.md` — PR descriptions, commit format, changelog policy, branch freshness, fix-vs-defer.
- `.claude/rules/pr-waste-patterns.md` — what counts as reviewer-distracting waste in a diff.
- `.claude/rules/no-personal-details.md` — keep personal/identifying details out of this public repo.
- `CONTRIBUTING.md` — contributor setup, scope, and PR posture.
- `.github/copilot-instructions.md` — review guidance for GitHub Copilot.

[bdd]: https://en.wikipedia.org/wiki/Behavior-driven_development
[tdd]: https://en.wikipedia.org/wiki/Test-driven_development
