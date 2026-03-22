# Changelog

## v1.7.2 — 2026-03-21

### Fixed

- **CI (GitHub Actions)** — Removed `GOMEMLIMIT=1GiB` (too tight for `go test ./...` with coverage; led to exit **143** / SIGTERM on runners). Use `GOMAXPROCS=2` and a 20-minute step timeout instead.
- **Homebrew tap** — Release workflow now copies `packaging/homebrew-bake.rb` in full each time, so `Formula/bake.rb` cannot accumulate merge-conflict markers from partial `sed` edits.

## v1.7.1 — 2026-03-14

Patch release: race-safety, CI reliability, and CLI ergonomics.

### Fixed

- **runner** — Concurrent `--json` / `EventWriter` output when `MaxParallel > 1` could race on a shared buffer. Writes are now serialized via a locked writer (`internal/runner/runner.go`).
- **daemonclient** — `EnsureStarted` waited only 3s for the socket; under `-race` or slow hosts the fake-daemon test could flake. Socket wait increased to 10s.
- **tests** — Watcher tests used a plain `reloadCount` updated from the debounce goroutine; switched to `atomic.Int32` for `-race`. Poll watch test timeout increased (1s) to reduce flakes.
- **GitHub Actions** — CI step sets `GOMEMLIMIT=1GiB` and `timeout-minutes: 15` to reduce OOM / exit 143 on runners.

### Changed

- **bake** — With `--ci` (or `CI=1`) and no target, run the **ci** suite instead of the default target (first target in the Bakefile was often wrong, e.g. `act`).
- **bake** — Suite runs print `bake: running suite …` and per-target lines; `--show-cmd` also prints each target’s step argv before execution; `--debug` prints suite entry list.
- **Bakefile (repo)** — `test.cover` preset renamed to `test.coverage`; precommit suite uses `test` instead of `test.no-cache` (full test cache allowed).

### Docs

- `docs/cli-reference.md` — Clarified `--show-cmd` and `--debug` for suite runs.

## v1.7.0

Initial **baked** daemon release (persistent queue, TCP, multi-workspace, hot reload, health checker, signals). See [GitHub releases](https://github.com/evmac/go-bake/releases).
