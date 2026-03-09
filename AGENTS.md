# Agent guide (go-bake)

Context for AI agents working on this repo.

## Project

**Bake** is a minimal Make replacement in Go. It runs targets and suites from a `Bakefile` (DSL), with incremental builds, profiles, and CLI overrides. Main entry: `cmd/bake`; core packages: `internal/config`, `internal/dsl`, `internal/runner`, `internal/resolve`, `internal/env`, `internal/cache`.

## Testing (required)

**Testing is mandatory.** Both integration and unit tests are required across the board.

- **TDD is the standard:** write or adjust tests first (or in lockstep), then implement. Do not ship behaviour without tests.
- **Coverage:** aim for **>80%** overall and **>90%** per package where practical. New features and bug fixes must include tests; use `go test ./... -cover` and `-coverprofile` / `-html` when adding or fixing code. See **@docs/testing-and-coverage.md** for targets and how to measure.
- **Full details:** see **@docs/testing-and-coverage.md** for how to run tests, measure coverage, where tests live, and how to write CLI vs integration vs unit tests.

When adding a feature (e.g. a new flag or DSL construct), add:

1. **Unit tests** for the new logic (parser, compiler, runner, or helpers).
2. **Integration tests** that exercise the behaviour via `RunMain(...)` or the relevant public API so the full path is covered.

## Docs and structure

- **Roadmap / scope:** [docs/future.md](docs/future.md)
- **Bakefile syntax:** [docs/bakefile-reference.md](docs/bakefile-reference.md), [docs/grammar-spec.md](docs/grammar-spec.md), [docs/llm-reference.md](docs/llm-reference.md)
- **CLI:** [docs/cli-reference.md](docs/cli-reference.md)
- **Parameterization (passthrough, overrides, presets):** [docs/parameterization.md](docs/parameterization.md)
- **Linting:** [docs/lint.md](docs/lint.md)
- **Releasing:** [docs/releasing.md](docs/releasing.md) — ship-it praxis (review → commit → tag → push → `gh release create`). No release script; follow the doc. Agents should not create or push tags/releases unless explicitly asked.

Tests live in `*_test.go` next to the code; CLI behaviour in `cmd/bake/main_test.go`. Use `t.TempDir()` for scratch dirs; avoid committing generated or temporary files.
