---
paths: ".github/workflows/*.yml"
---

# GitHub Actions Pinning

Pin every `uses:` action to a full commit SHA, with the human-readable version as a trailing comment:

```yaml
- uses: actions/checkout@df4cb1c069e1874edd31b4311f1884172cec0e10 # v6
```

This holds for **all** actions — GitHub-authored and third-party alike — so the supply chain is uniform and auditable: a tag can be repointed, a commit SHA cannot. The `github-actions` Dependabot ecosystem bumps the SHAs and updates the version comment, so pinning does not freeze versions.

When adding or updating an action, resolve the tag to its SHA rather than using the bare tag:

```sh
gh api repos/<owner>/<repo>/commits/<tag> --jq '.sha'
```
