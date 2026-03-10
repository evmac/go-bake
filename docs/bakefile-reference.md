# Bakefile reference

## Overview

A Bakefile is a custom DSL. Keywords: `target`, `deps`, `steps`, `exec`, `cmd`, `shell`, `env`, `args`, `dotenv`, `suite`, `profile`, `import`, `preset`, `cwd`, `desc`, `tags`, `passthrough`, `inputs`, `outputs`, `when`, `private`, `pool`, `mutex`, `image`, `unsafe`, `net`, `vol`. A suite named **`precommit`** is used by **`bake install hooks`** (see [Installing git hooks](install-hooks.md)). Comments: `#` to end of line. Bake automatically formats and lints the Bakefile when loading (unless `BAKE_NO_AUTOFORMAT` / `BAKE_NO_AUTOLINT`); you can also run **`bake format`** or **`bake fmt`** (with **`-w`**) and **`bake lint`** (with **`--fix`**) manually.

## File structure

- Optional **dotenv** line: `dotenv .env .env.local`
- One or more **target**, **suite**, or **profile** blocks (and optional **import**, **private**)

**Formatter order** — **`bake format`** (and auto-format on load) writes file entries in a canonical order: **suites** (alphabetically by name), then **profiles** (alphabetically by name), then **targets** (alphabetically by name). Within each suite, the list of targets is also sorted by name. Imports and the file-level **private** marker stay before suites.

## Targets

**Bracketed form (recommended):**

```bake
target build {
  deps generate
  steps { exec ["go", "build", "-o", "bin/app", "./cmd/app"] }
  env { GOOS linux GOARCH amd64 }
  cwd cmd/app
  desc "build the binary"
}
```

**Single step** — When a target has only one command, you can omit the `steps { }` wrapper and write `exec` or `cmd` directly:

```bake
target format { exec ["go", "fmt", "./..."] }
target fmt { cmd go fmt ./... }
```

**Single-line form** (only when the file has exactly one target and one step):

```bake
target build cmd go build ./...
```

## Steps

- **exec** — canonical: explicit argv list. `exec ["go", "test", "./..."]`
- **cmd** — sugar: space-separated tokens. `cmd go test ./...`
- **shell** — run a script with a shell. `shell bash "go test ./... | tee out"`

Steps run in order. **Passthrough:** `passthrough step=N` (optional `name "id"` for tooling); args after `--` on the CLI are appended to that step's argv. You can declare multiple passthrough steps (e.g. `passthrough step = 1` and `passthrough step = 3`); then split CLI args by `--`: `bake target -- a b -- c` sends `[a,b]` to step 1 and `[c]` to step 3. For how passthrough relates to overrides and target presets, see [Parameterization](parameterization.md).

## Inputs and outputs (incremental build)

Declare file inputs and outputs so bake can skip the target when nothing changed:

```bake
target build {
  inputs [ "*.go", "go.mod" ]
  outputs [ "bin/app" ]
  steps { exec ["go", "build", "-o", "bin/app", "./..."] }
}
```

- Paths are relative to the Bakefile root; globs are supported (`*`, `**` in future).
- Cache is stored under `.bake/cache/`. If all inputs have the same content hash as the last run and all outputs exist, the target is skipped.
- Use `bake --why <target>` to see why a target would run or be skipped (changed inputs, missing outputs, or cache hit).

## Conditionals (when)

Run a target only when a condition holds; otherwise the target is skipped (no-op, but still counts as success for dependents). Dependencies are always run first.

- **when env VAR** — run only if the environment variable `VAR` is set (non-empty). Example: `when env CI` to run only in CI.
- **when cmd ["prog", "args"]** — run only if the command exits 0. Example: `when cmd ["test", "-f", "Makefile"]` to run only when Makefile exists.

```bake
target test {
  when env CI
  steps { exec ["go", "test", "./..."] }
}

target legacy {
  when cmd ["test", "-f", "Makefile"]
  steps { exec ["make", "build"] }
}
```

## Deps

`deps name1, name2` — run these targets before this one (comma-separated). Order is topological.

## Env

- **dotenv** — load `.env` (and optional files) from project root before any target.
- **env** block — per-target overlay: `env { KEY value }`. Values can be ident or quoted string.

## Args

Declare target arguments for CLI and interpolation:

```bake
target deploy {
  args env string -e staging
  args dry_run bool false
  steps { exec ["deploy.sh", "{{.env}}", "{{.dry_run}}"] }
}
```

- `{{.argName}}` in steps (argv, env) is replaced by the value.
- Undeclared `--key value` on the CLI are "live args"; use `{{.live.key}}` in the Bakefile (optional).

## Target presets

A preset modifies a target's execution. It is not a sub-command or a separate target — it applies overrides to the base target. Each preset can include any combination of four knobs:

| Knob | Effect |
|------|--------|
| **desc** | Alternate description (for `--list`, `--explain`). |
| **argv** | Extra arguments appended to the passthrough step. |
| **env** | Environment variable overlay (merged on top of target env). |
| **steps** | Complete step replacement (ignores `argv`; use when the command is fundamentally different). |

Use **`argv`** or **`env`** when the preset only adds flags or changes environment. Use **`steps`** when the command itself is different — duplication is intentional and keeps each preset explicit.

```bake
target test {
  desc "run tests"
  steps { exec ["go", "test", "./..."] }
  preset cover {
    desc "run tests with coverage"
    argv ["-coverprofile=coverage.out"]
  }
  preset race {
    desc "run tests with race detector"
    env { GORACE "halt_on_error=1" }
    argv ["-race"]
  }
}

target format {
  desc "format Go code"
  steps { exec ["go", "fmt", "./..."] }
  preset check {
    desc "check formatting (no auto-fix)"
    steps { exec ["go", "fmt", "-l", "./..."] }
  }
}
```

- **`bake test`** — runs the base target (go test ./...).
- **`bake test cover`** — appends `-coverprofile=coverage.out` to the test step.
- **`bake test race`** — sets GORACE env and appends `-race`.
- **`bake format check`** — replaces steps entirely with `go fmt -l ./...`.

**Invocation:** **`bake <target> [preset]`**. In suites, use **dot notation**: `test.cover`, `format.check`.

## Suites

Suite entries are space- or newline-separated. Each entry is a target name or **target.preset** (dot notation):

```bake
suite dev { build test deploy }
suite ci { build lint test format.check test.cover }
```

- Single word: `build`, `test`. Target with preset: `format.check`, `test.cover`. Quoted strings are also allowed (e.g. `"test cover"`); the formatter normalizes to dot form.
- **Default suite for CI** — A suite named **`ci`** is the conventional entry point for CI: **`--list --ci`** and **`bake ci`** use it when **`CI=1`** or **`--ci`** is set. Define **`suite ci { ... }`** with the targets you want to run in CI (e.g. build, lint, test); then in your CI config run **`bake ci`** or **`bake --ci`**.
- **Default suite for local** — Without `--ci`, **`--list`** and the default target use the **`dev`** suite when present. Running **`bake ci`** runs all targets in the `ci` suite.

## Pool and mutex

- **pool** *name* — target uses this named pool (semaphore). Targets sharing a pool name run with limited concurrency (default 1 at a time per pool).
- **mutex** *name* — target holds this mutex while running. Only one target holding a given mutex name runs at a time.

Use with **`--max-parallel`** to run independent targets in parallel while limiting contention (e.g. `pool docker` for targets that use Docker).

## Image, unsafe, net, and vol (containerization)

- **image** *ref* — run this target’s steps inside a container using the given image (e.g. `image "alpine:3.19"` or `image alpine`). Docker is the default runtime. Requires Docker (or compatible daemon) to be available; otherwise bake reports an error.
- **unsafe** — run this target on the host even when **image** is set (opts out of the container sandbox).
- **net** *name* — attach the container to this named network. The first target (in execution order) that references a name creates the network; later targets attach to the same one.
- **vol** *name* or **vol** *name* *hostPath* — mount a named volume or bind-mount. **vol** *name* creates a Docker named volume at first reference; **vol** *name* *hostPath* bind-mounts the given host path (relative to Bakefile root) into the container at `/mnt/<name>`.

**Example:**

```bake
target in-docker {
  image "alpine:3.19"
  net mynet
  vol cache
  steps { exec ["sh", "-c", "echo hello"] }
}

target unsafe-on-host {
  image alpine
  unsafe
  steps { exec ["echo", "runs on host"] }
}
```

Image pull behavior can be set with **`BAKE_PULL`**: `always`, `never`, or `if-not-present` (default).

## Precommit (hooks)

Define a **suite named `precommit`** to control what runs in the git pre-commit hook. When **`suite precommit { ... }`** is present, **`bake install hooks`** writes `.git/hooks/pre-commit` to run **`bake precommit`** (all targets in that suite). If you don’t define this suite, hooks are not installed. See [Installing git hooks](install-hooks.md) for usage and examples.

## Cwd, desc, tags

- **cwd** — working directory for the target (relative to Bakefile root).
- **desc** — one-line description for `--list` and help.
- **tags** — optional list of tags for grouping.
