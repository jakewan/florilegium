# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.1.0] - 2026-06-12

### Added

- Load configuration from `$XDG_CONFIG_HOME/florilegium/config.yml` (corpus path and recency window) at startup; missing or malformed config fails with a clear message instead of panicking.
- Serve the corpus through the `list_candidates`, `record_use`, and `list_tags` MCP tools: `list_candidates` returns a shortlist (id, text, meta, tags) in corpus order, narrowed to items sharing any of the requested tags and excluding ids used within the recency window (override with `exclude_recent: false`, cap with `limit`); `record_use` marks an item used so it drops out of the next shortlist and rejects an unknown id with a clear error; `list_tags` returns the distinct tags, deduplicated and sorted. The server applies no relevance ranking — choosing among candidates is the caller's job.
- Exit cleanly (status 0) when the connected client disconnects, treating the normal end of a session as success rather than reporting it as an error.
- Override the config-file path with a `--config` flag or the `FLORILEGIUM_CONFIG` environment variable (precedence: flag, then env, then the XDG default), so an instance can be pointed at its own config by a single florilegium-specific knob rather than by relocating the general-purpose `XDG_CONFIG_HOME`.
- Set the history-log path with an optional `history:` field in the config; when omitted it defaults to `$XDG_STATE_HOME/florilegium/history.jsonl` as before. Naming a per-corpus config (via `FLORILEGIUM_CONFIG`) that carries its own `history:` isolates a multi-corpus setup's recency through one knob per instance.
- Validate the corpus against format version 1: an optional top-level `version:` field declares the format a corpus targets (omit it to accept the baseline), and a corpus naming an unsupported version is rejected with an actionable error instead of loading into wrong selection behavior. Unknown or misspelled fields are now rejected at load time rather than silently dropped, and a file carrying more than one YAML document is rejected rather than loading only the first — so a stale, mistyped, or malformed corpus fails loudly.
- Report a build-stamped version to connecting MCP clients, derived from the release tag at build time (an untagged build reports `dev`), so a release identity travels with the binary.

### Changed

- Corpus items now carry an opaque `meta` key/value map instead of a dedicated `attribution` field; the server stores and returns `meta` verbatim and never interprets or queries it. Use conventional keys like `attribution` and `source` as you see fit; `tags` remains the queryable axis. Migration: move any `attribution:` value under a `meta:` block — a leftover `attribution:` field is now rejected at load time (it is an unknown field), so migrate before upgrading.
- Trim the history log at startup so it no longer grows without bound; the retained tail stays larger than the recency window, so trimming never changes which items are eligible.

[Unreleased]: https://github.com/jakewan/florilegium/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/jakewan/florilegium/releases/tag/v0.1.0
