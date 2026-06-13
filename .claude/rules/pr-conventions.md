# PR and Commit Conventions

How pull requests, commits, and the changelog work in this project.

## PR Descriptions

(extension point: `pr-description-format`)

Structure a PR body as:

- **Overview** — the purpose: what the change accomplishes and why. Lead with this.
- **How it works** (optional) — only for non-obvious mechanics a reviewer cannot infer from the diff.
- **Issue references** — closing keywords (`Closes #N`); repeat the keyword for each issue (`Closes #1, closes #2`).

Avoid:

- Enumerating the diff file-by-file — the diff already shows what changed.
- Narrating the drafting journey ("earlier this did X, then I changed it").
- Scaffolding headers with no content under them.
- Hard-wrapping prose at a fixed column. Write one long line per paragraph and let the renderer wrap.

## Commit Messages

(extension point: `squash-commit-format`)

Conventional Commits: `type(scope): subject`.

- **Types**: `feat`, `fix`, `refactor`, `docs`, `build`, `ci`, `test`, `chore`.
- **Scopes** (this project's areas): `mcp`, `corpus`, `history`, `config`, `ci`, `build`, `docs`, `rules`, `dx`, `deps`. `dx` covers developer-experience work (tooling, justfile, hooks); `deps` covers dependency updates (e.g., Dependabot bumps).
- **Body**: short prose stating *why* — the motivation, constraint, or problem solved — sized to the change, not its diff. A small change may need a one-line body or none. Don't restate the diff, don't narrate the journey ("the review surfaced...", "earlier this did X"), and don't re-derive rationale a durable doc (a design decision in `CLAUDE.md`, a doc comment) already records — point to it or omit it. State the durable *why* once, concisely. The body of a user-facing commit is the **changelog source** (see Changelog), so write it to read for a user, not only a reviewer.
- **Breaking changes**: mark with a `!` after the type/scope (`feat(corpus)!: …`) or a `BREAKING CHANGE:` footer. This drives the version bump (a breaking change bumps the minor while the project is pre-1.0) and flags the change in the changelog. It is the commit-level complement to the corpus format version (an incompatible corpus shape ships with both a format-version bump and a breaking-marked commit).
- **Issue references**: `Closes #N` (or `Related to #N`); repeat the keyword per issue. These trailers are stripped from the generated changelog.

## Changelog

(extension point: `changelog-convention`)

`CHANGELOG.md` follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and [Semantic Versioning](https://semver.org/spec/v2.0.0.html), and is **generated from the Conventional Commit history** by [git-cliff](https://git-cliff.org) at release time (config in `cliff.toml`). The commit — its `type(scope): subject` and its why-body — is the changelog source.

**Do not hand-add an `[Unreleased]` entry per PR.** Write a clear conventional subject and a user-facing why-body (see Commit Messages); the body is rendered into the changelog as the entry's prose. What lands follows the commit type: `feat` → Added, `fix` → Fixed, `refactor`/`perf` → Changed; `docs`/`test`/`ci`/`build`/`chore` are omitted as non-user-facing, and a breaking-marked commit is flagged. Issue trailers and co-author lines are stripped.

The existing hand-written `## [0.1.0]` section is preserved; git-cliff prepends new versions above it.

### Release ritual

A pushed `v*` tag is the single release gate — it triggers the workflow that cross-compiles, signs (cosign keyless), attests (SLSA provenance), and publishes the GitHub Release with notes extracted from the tag's `CHANGELOG.md` section. To cut a release:

1. `main` clean and CI green; **`git fetch --tags`** so the bump computes against the latest tag.
2. `git cliff --bumped-version` to see the next version (or choose it), then `git cliff --bump --prepend CHANGELOG.md` to generate the new section. **Review and polish the prose** — the generated draft renders full commit bodies and is usually worth tightening.
3. Commit `chore(release): vX.Y.Z` (this type is excluded from the changelog).
4. Tag `vX.Y.Z` and push the tag.

`just changelog` previews the unreleased section; `just release-check` validates the release config and dry-runs a snapshot build locally.

The first release (`v0.1.0`) is a one-off: its `[0.1.0]` section is already hand-written, so it skips steps 2–3 — just tag and push.

## Branch Freshness

(extension point: `freshness-response-policy`)

When assessing how far a branch trails its base:

- **Up to date** — proceed.
- **Modestly behind** — note it; refreshing is optional.
- **Significantly behind, or conflicts are likely** — merge the base branch in first (this project merges, never rebases) before merging the PR.

## Code-Audit Fix vs. Defer

(extension point: `code-audit-practice`)

When acting on review findings:

- **Fix in place** when the finding is small, localized, and within the PR's stated purpose.
- **Defer to a GitHub issue** when addressing it would expand the PR's scope or is tangential to its purpose.
