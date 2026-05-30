---
paths: "**/*.go"
---

# Go Practices

Conventions for Go code in this repository. These load when editing Go files.

## Errors

- Wrap errors with context as they propagate: `fmt.Errorf("loading corpus: %w", err)`. Use `%w` so callers can `errors.Is`/`errors.As` the cause.
- When matching a sentinel error from a dependency, confirm it is actually reachable — `errors.Is` only traverses causes wrapped with `%w`, so a cause formatted with `%v` (or otherwise absent from the chain) silently fails to match. Verify the wrapping, or match a stable error code or type, rather than assuming `errors.Is` works.
- Return errors; don't `log.Fatal` outside `main`. The single acceptable fatal is the top-level server-run error in `main`.
- Make validation errors specific and actionable — name what was wrong (which id, which field), so the message stands on its own.
- When validating a required string field as non-empty, trim whitespace first (`strings.TrimSpace(s) == ""`) so a whitespace-only value is rejected consistently with an absent one — a blank-looking value should not pass a check that a missing one fails.

## Context

- Functions that do I/O or are cancellable take `context.Context` as the **first** parameter.
- Don't store a `context.Context` in a struct; pass it through the call chain.

## Output discipline (MCP over stdio)

This server speaks JSON-RPC over stdio. **stdout is the protocol stream — write nothing else to it.** No `fmt.Println`/`fmt.Printf` to stdout in server code. Diagnostics go to stderr via `log`.

## Tests

- Use the standard `testing` package — no external assertion or mocking frameworks.
- Prefer table-driven tests for behavior variations (valid input, invalid input, edge cases, error paths) — not just the happy path.
- Isolate filesystem and history state with `t.TempDir()`; register cleanup with `t.Cleanup`.
- Tests describe **what** the code does from the caller's perspective, not **how** it does it internally. Interfaces should exist because a test needs to substitute an implementation, not as speculative abstraction.

## Documentation

- Exported types, functions, and packages carry godoc comments. Begin with the symbol's name (`// Open opens …`).
- Comments explain **why** — rationale, constraints — not **what** the code already says.
