# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Load configuration from `$XDG_CONFIG_HOME/florilegium/config.yml` (corpus path and recency window) at startup; missing or malformed config fails with a clear message instead of panicking.
- Register the `list_candidates`, `record_use`, and `list_tags` MCP tools so a connected client can enumerate them. Handlers are stubbed pending later work.
- Exit cleanly (status 0) when the connected client disconnects, treating the normal end of a session as success rather than reporting it as an error.
