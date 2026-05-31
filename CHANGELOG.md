# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Load configuration from `$XDG_CONFIG_HOME/florilegium/config.yml` (corpus path and recency window) at startup; missing or malformed config fails with a clear message instead of panicking.
- Serve the corpus through the `list_candidates`, `record_use`, and `list_tags` MCP tools: `list_candidates` returns a shortlist (id, text, meta, tags) in corpus order, narrowed to items sharing any of the requested tags and excluding ids used within the recency window (override with `exclude_recent: false`, cap with `limit`); `record_use` marks an item used so it drops out of the next shortlist and rejects an unknown id with a clear error; `list_tags` returns the distinct tags, deduplicated and sorted. The server applies no relevance ranking — choosing among candidates is the caller's job.
- Exit cleanly (status 0) when the connected client disconnects, treating the normal end of a session as success rather than reporting it as an error.

### Changed

- Corpus items now carry an opaque `meta` key/value map instead of a dedicated `attribution` field; the server stores and returns `meta` verbatim and never interprets or queries it. Use conventional keys like `attribution` and `source` as you see fit; `tags` remains the queryable axis. Migration: move any `attribution:` value under a `meta:` block — a legacy `attribution:` field is now silently ignored (loaded as no meta), so update at your convenience.
- Trim the history log at startup so it no longer grows without bound; the retained tail stays larger than the recency window, so trimming never changes which items are eligible.
