# Contributing to Florilegium

## Issues

This project uses **problem-framed issues**. The issue template asks you to describe:

- **The problem** you're experiencing
- **Current behavior** — what happens today
- **Desired behavior** — what you'd expect instead
- **Why it matters** — the impact on your workflow

Focus on describing the problem clearly. Solution ideas are welcome as supplementary context, but the issue should stand on the strength of the problem description alone.

## Scope

Florilegium is a single-purpose MCP server: it surfaces one apt item at a time from a user-supplied corpus — recency-aware, without recent repeats — and leaves the question of *which* item fits to the calling agent. It does no relevance ranking of its own.

Contributions should stay within this focused scope. If you're unsure whether something fits, open an issue describing the problem first.

## Naming and capitalization

The project name carries three registers; keep them distinct so capitalization doesn't drift across the docs:

| Register | Form | Where it appears |
| --- | --- | --- |
| **Brand** (proper noun) | `Florilegium` (Title-Case) | Prose, sentence-leading mentions, headings, and the MCP display title clients show |
| **Latin common noun** | *florilegium* (lowercase, italic) | When referring to the *word* or the anthology concept it names, not the product (e.g. the README's etymology gloss) |
| **Machine identifier** | `florilegium` (lowercase) | The binary, the Go package and import path (`github.com/jakewan/florilegium`), the `mcpServers` config key, `$FLORILEGIUM_CONFIG`, config and state paths, the `log` prefix, and the registry name |

In code, this is the `serverName` (lowercase identifier) / `serverTitle` (Title-Case display name) split registered on the MCP server's `Implementation`.

## Development

### Setup

Tool versions are managed by [mise](https://mise.jdx.dev/). After cloning:

```bash
mise install        # Install Go, golangci-lint, just, lefthook
just hooks          # Install git hooks (lefthook)
```

### Build, Test, Lint

All commands go through [just](https://github.com/casey/just):

```bash
just build    # Build binary to bin/
just test     # Run all tests
just lint     # Run golangci-lint
just install  # Install the binary to ~/.local/bin
```

### Testing Approach

The project uses BDD-style/outside-in TDD:

- Write failing tests before production code.
- Drive the MCP tool surface from acceptance tests that exercise the server, then build inward.
- Tests use the standard `testing` package — no external test frameworks.
- Use table-driven tests for multiple scenarios; isolate filesystem and history state with `t.TempDir()`.

## Pull Requests

- Keep PRs small and focused — each PR should serve a single purpose.
- PRs are squash-merged, so commit history within a branch doesn't need to be pristine.
- This project merges, never rebases.
