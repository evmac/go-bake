# Releasing

Ship-it praxis: human review → commit → tag → push → release via GitHub.

Release notes for each version: [changelog](changelog.md).

The Homebrew formula source lives at [`packaging/homebrew-bake.rb`](../packaging/homebrew-bake.rb); the **Update Homebrew tap** workflow copies it on each release so the tap stays conflict-free.

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

## Pre-commit

We don’t use the [pre-commit](https://pre-commit.com/) framework. Before pushing (or before commit), run:

```bash
bake ci
bake lint
```

Use **`bake install hooks`** to install a git pre-commit hook that runs **`bake precommit`** on every commit; see [Installing git hooks](install-hooks.md).

## Release checklist

Before you consider a release done, make sure you've **actively completed each step** below:

- [ ] Run `bake precommit` and ensure all checks pass (formatting, lint, unit/integration tests).
- [ ] Version: the binary reports it via **`bake --version`**. For release builds, set it at build time with `-ldflags "-X main.Version=v1.x.0"` (e.g. when building release binaries or in the Homebrew formula). No need to edit source each release.
- [ ] Double-check that your git tag (e.g., `v1.4.0`) matches the release version.
- [ ] Push both the branch and the tag to GitHub to trigger the release workflow. Ensure the GitHub Actions workflow (`release: published`) runs and the Homebrew tap is updated automatically.

**Tip:** If any step fails, fix the issue, recommit, re-tag if necessary, and retry. Don't assume automation will correct mistakes!
