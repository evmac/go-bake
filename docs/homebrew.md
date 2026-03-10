# Publishing to Homebrew

You can distribute `bake` via Homebrew in two ways.

## Option 1: Personal tap (recommended, no approval needed)

Users install with:

```bash
brew tap evmac/bake
brew install bake
```

### Steps

1. **Create a tap repo** on GitHub named `homebrew-bake` (or `homebrew-tap` if you host multiple formulae). By convention, the tap name is `evmac/bake` for `github.com/evmac/homebrew-bake`.

2. **Tag a release** in this repo (e.g. `v1.0.0`). The Formula uses the tarball URL:
   `https://github.com/evmac/go-bake/archive/refs/tags/v1.0.0.tar.gz`

3. **Add the formula** in your tap repo (evmac/homebrew-bake):
   - Clone: `git clone https://github.com/evmac/homebrew-bake`
   - Add `Formula/bake.rb` (or `Formula/b/bake.rb` if you follow core’s subdir layout) with `url` set to the release tarball and `sha256` from:
     ```bash
     curl -sL "https://github.com/evmac/go-bake/archive/refs/tags/v1.0.0.tar.gz" | shasum -a 256
     ```
   - Commit and push.

4. **On new releases**: update `url` and `sha256` in the formula and push. Alternatively, **automatic updates** are handled by the workflow in `.github/workflows/release-homebrew.yml`: when you publish a release in this repo, it updates the formula in the tap and pushes. That workflow requires the repo secret **`HOMEBREW_TAP_TOKEN`** (a PAT with Contents read/write on evmac/homebrew-bake; classic PAT: `repo` scope).

### Head install (no tag)

For the latest from `main`:

```bash
brew install --HEAD evmac/bake/bake
```

(Requires `head "https://github.com/evmac/go-bake.git", branch: "main"` in the formula in the tap.)

---

## Option 2: homebrew-core (official)

Install with `brew install bake` (no tap). Requires a PR to [Homebrew/homebrew-core](https://github.com/Homebrew/homebrew-core) and maintainer approval. They expect:

- Notable, stable project with a tagged release
- [Acceptable Formulae](https://docs.brew.sh/Acceptable-Formulae) criteria (e.g. 30+ stars, or clear notability)

To submit:

1. Tag a release (e.g. `v1.0.0`).
2. Run `brew create https://github.com/evmac/go-bake/archive/refs/tags/v1.0.0.tar.gz` and adapt the generated formula (e.g. using the tap’s formula as reference).
3. Add a proper `test do` block (see existing formula).
4. Run `brew audit --strict --new --online bake` and `brew test bake`.
5. Open a PR to homebrew-core with the formula in `Formula/b/bake.rb`.

---

## Formula location

The formula lives in the **tap** (evmac/homebrew-bake), in `Formula/bake.rb`. Create and maintain it there; set `url` and `sha256` to the release tarball for each version you ship (or rely on the release workflow to update those two fields automatically).

**Version in the binary:** To have `bake --version` print the release version (e.g. `v1.6.0`), the formula’s `go build` step should pass ldflags, e.g. `-ldflags "-X main.Version=#{version}"` (Ruby) so the version matches the tag. If the formula doesn’t set this, `bake --version` will show `dev`.
