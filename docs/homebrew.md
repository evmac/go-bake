# Publishing to Homebrew

You can distribute `bake` via Homebrew in two ways.

## Option 1: Personal tap (recommended, no approval needed)

Users install with:

```bash
brew tap em/bake
brew install bake
```

### Steps

1. **Create a tap repo** on GitHub named `homebrew-bake` (or `homebrew-tap` if you host multiple formulae). By convention, the tap name is `em/bake` for `github.com/em/homebrew-bake`.

2. **Tag a release** in this repo (e.g. `v1.0.0`). The Formula uses the tarball URL:
   `https://github.com/em/go-bake/archive/refs/tags/v1.0.0.tar.gz`

3. **Copy the Formula** from `Formula/bake.rb` into your tap repo:
   - Clone: `git clone https://github.com/em/homebrew-bake`
   - Add: `Formula/bake.rb` (or `Formula/b/bake.rb` if you follow core’s subdir layout)
   - Set `url` to the release tarball and update `sha256`:
     ```bash
     curl -sL "https://github.com/em/go-bake/archive/refs/tags/v1.0.0.tar.gz" | shasum -a 256
     ```
   - Commit and push.

4. **On new releases**: update `url` and `sha256` in the formula and push.

### Head install (no tag)

For the latest from `main`:

```bash
brew install --HEAD em/bake/bake
```

(Requires `head "https://github.com/em/go-bake.git", branch: "main"` in the formula, which is already there.)

---

## Option 2: homebrew-core (official)

Install with `brew install bake` (no tap). Requires a PR to [Homebrew/homebrew-core](https://github.com/Homebrew/homebrew-core) and maintainer approval. They expect:

- Notable, stable project with a tagged release
- [Acceptable Formulae](https://docs.brew.sh/Acceptable-Formulae) criteria (e.g. 30+ stars, or clear notability)

To submit:

1. Tag a release (e.g. `v1.0.0`).
2. Run `brew create https://github.com/em/go-bake/archive/refs/tags/v1.0.0.tar.gz` and adapt the generated formula using `Formula/bake.rb` as reference.
3. Add a proper `test do` block (see existing formula).
4. Run `brew audit --strict --new --online bake` and `brew test bake`.
5. Open a PR to homebrew-core with the formula in `Formula/b/bake.rb`.

---

## Formula location in this repo

The template formula lives at **`Formula/bake.rb`** in this repo. Use it as the source for your tap (or as reference for a homebrew-core PR). Remember to set the `url` and `sha256` to the release tarball for the version you ship.
