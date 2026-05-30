---
paths: "**/*.go"
---

# Go Practices

Conventions for Go code in this repository.

## Errors

- Wrap errors with context as they propagate: `fmt.Errorf("loading corpus: %w", err)`. Use `%w` so callers can `errors.Is`/`errors.As` the cause.
- Before relying on `errors.Is` to match a dependency's sentinel, confirm the cause is in the chain — `errors.Is` only traverses causes wrapped with `%w`, so one formatted with `%v` silently fails to match. When unsure, match a stable error code or type instead.
- Return errors; don't `log.Fatal` outside `main`. The single acceptable fatal is the top-level server-run error in `main`.
- Make validation errors specific and actionable — name what was wrong (which id, which field) so the message stands on its own.
- Trim whitespace before checking a required string is non-empty (`strings.TrimSpace(s) == ""`), so a blank-looking value is rejected like a missing one.

## Context

- Functions that do I/O or are cancellable take `context.Context` as the **first** parameter.
- Don't store a `context.Context` in a struct; pass it through the call chain.

## MCP server

This server speaks JSON-RPC over stdio.

- **stdout is the protocol stream — write nothing else to it.** No `fmt.Println`/`fmt.Printf` to stdout in server code; diagnostics go to stderr via `log`.
- Publish a tool's input constraints — defaults, bounds (`minimum`/`maximum`), required vs optional — in its JSON schema, not in handler code. The schema is the contract callers introspect, and the SDK enforces it before the handler runs, so invalid input fails with a clear validation error instead of being silently tolerated. (Worked examples: the `exclude_recent` default and `limit` minimum on `list_candidates` in `internal/server`.)
- When a tool's input schema carries any default, guard against a literal-null arguments payload. A client sending `"arguments": null` makes the SDK apply the default into a nil map and panic, ending the session. Register a receiving middleware that rewrites a null payload to absent so the defaults apply as if no arguments were passed (`tolerateNullArguments` in `internal/server`).

## Tests

- Use the standard `testing` package — no external assertion or mocking frameworks.
- Prefer table-driven tests for behavior variations (valid input, invalid input, edge cases, error paths) — not just the happy path.
- Isolate filesystem and history state with `t.TempDir()`; register cleanup with `t.Cleanup`.
- Tests describe **what** the code does from the caller's perspective, not **how**. An interface should exist because a test needs to substitute an implementation, not as speculative abstraction.
- Exercise tool behavior through an in-memory client/server session (`mcp.NewInMemoryTransports`), asserting on the structured result — and on `IsError` for the error paths.

## Documentation

- Exported types, functions, and packages carry godoc comments beginning with the symbol's name (`// Open opens …`).
- Comments explain **why** — rationale, constraints — not **what** the code already says.
