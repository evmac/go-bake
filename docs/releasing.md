# Releasing

Ship-it praxis: human review → commit → tag → push → release via GitHub.

## Prerequisites

- [GitHub CLI](https://cli.github.com/) (`gh`) installed and authenticated.
- Clean working tree (commit or stash changes).
- CI green on the branch you’re releasing from (e.g. `trunk`).

## Steps

1. **Human review**  
   Complete code review and any final edits. Run the test suite and lint:

   ```bash
   bake ci
   bake lint
   ```

2. **Commit the feature set**  
   Commit all changes for the release (e.g. `v1.4.0`):

   ```bash
   git add -A
   git commit -m "Release v1.4.0: presets, passthrough, concurrency, watch, lint, CI/JSON"
   ```

3. **Tag the commit**  
   Use semantic versioning (e.g. `v1.4.0`):

   ```bash
   git tag -a v1.4.0 -m "v1.4.0"
   ```

4. **Push to remote**  
   Push the branch and the tag:

   ```bash
   git push origin trunk
   git push origin v1.4.0
   ```

5. **Create the GitHub release**  
   This publishes the release; the [Homebrew tap workflow](.github/workflows/release-homebrew.yml) runs on `release: published` and updates the formula.

   ```bash
   gh release create v1.4.0 --generate-notes
   ```

   Or with a custom title/notes:

   ```bash
   gh release create v1.4.0 --title "v1.4.0" --notes-file CHANGELOG.md
   ```

   To attach binaries (e.g. built elsewhere), add paths at the end:

   ```bash
   gh release create v1.4.0 --generate-notes bin/bake-linux-amd64 bin/bake-darwin-arm64
   ```

## Pre-commit

We don’t use the [pre-commit](https://pre-commit.com/) framework. Before pushing (or before commit), run:

```bash
bake ci
bake lint
```

Use **`bake install hooks`** to install a git pre-commit hook that runs **`bake precommit`** on every commit; see [Installing git hooks](install-hooks.md).

## Checklist

- [ ] `bake ci` passes
- [ ] `bake lint` passes (or acceptable findings)
- [ ] Version and docs (e.g. `docs/future.md`) updated
- [ ] Tag matches intended version (e.g. `v1.4.0`)
- [ ] Pushed tag triggers release; Homebrew tap updates automatically on `release: published`
