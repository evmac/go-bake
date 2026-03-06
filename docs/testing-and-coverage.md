# Testing and coverage

How we run tests, measure coverage, and what to do when something breaks.

## Running tests

**From the repo root:**

```bash
bake test
```

This builds the binary (if needed) and runs `go test ./...`. Use the **dev** suite for day-to-day work:

```bash
bake dev      # build, format, test
bake test     # build + test only
```

**Direct Go:**

```bash
go test ./...
```

Useful variants:

- `go test ./internal/runner/... -v -run TestRunWithCache` — one package, verbose, filter by test name
- `go test ./... -count=1` — disable cache (fresh run)
- `go test ./... -short` — skip long tests (if we add any and gate them with `testing.Short()`)

## Coverage

**Quick summary:**

```bash
go test ./... -cover
```

**Per-function breakdown:**

```bash
go test ./... -coverprofile=coverage.out
go tool cover -func=coverage.out
```

**HTML report (find uncovered lines):**

```bash
go test ./... -coverprofile=coverage.out
go tool cover -html=coverage.out
# opens in browser; red = uncovered
```

**Coverage targets:**

- **Overall:** aim for **>80%** coverage across all packages (`go test ./... -cover`).
- **Per package:** aim for **>90%** where practical (e.g. `internal/dsl`, `internal/runner`, `internal/config`, `internal/resolve`, `internal/cache`). Use coverage as a guide when adding or changing code.
- **cmd/bake:** tests are integration-level only (RunMain, real Bakefiles, exit codes). It’s fine for this package to sit below the usual per-package threshold; focus on covering behaviour rather than a number.

**Praxis:**

- Run `-cover` before pushing to spot regressions.
- Use `-html` when adding a feature or fixing a bug to see which branches you missed.
- Coverage is produced in CI (see below); check the workflow run or artifact if you want numbers after a push.

## Writing tests

- **Package tests** live in `*_test.go` next to the code. Use `t.TempDir()` for scratch dirs; avoid leaving files in the repo.
- **CLI behaviour** is tested in `cmd/bake/main_test.go` by calling `RunMain(args)` after `os.Chdir` into a temp dir that has a Bakefile. That exercises flag parsing and dispatch without exec’ing the binary.
- **Integration-style** tests (e.g. `TestBakeListExitCode`) build the binary and run it in a subprocess when you need real `main()` or exit codes.
- **Tables / subtests** — use `t.Run("name", func(t *testing.T) { ... })` for grouped cases (e.g. `TestWhyReasons`).

When something breaks, add a test that reproduces the bug (or the missing behaviour), then fix. Prefer one focused test per behaviour.

## CI

The **CI** workflow runs the **bake ci** suite: it checks out the repo, runs `go run ./cmd/bake ci`, which runs `build` then **test-cover** (tests with `-coverprofile=coverage.out`). So CI is “bake’s own ci suite”; no separate script.

The workflow uploads `coverage.out` as an artifact; download it and run `go tool cover -html=coverage.out` locally if you want to inspect.

To run the same as CI locally:

```bash
go run ./cmd/bake ci
```

That produces `coverage.out` in the repo root (and is in `.gitignore`). For a quick test run without coverage, use `bake test`.
