# Bakefile reference

## Overview

A Bakefile is a custom DSL. Keywords: `target`, `deps`, `steps`, `exec`, `cmd`, `shell`, `env`, `args`, `dotenv`, `suite`, `cwd`, `desc`, `tags`, `passthrough`, `inputs`, `outputs`, `when`. Comments: `#` to end of line. Run **`bake format`** or **`bake fmt`** (optionally with **`-w`**) to normalize indentation and clause order.

## File structure

- Optional **dotenv** line: `dotenv .env .env.local`
- One or more **target** or **suite** blocks

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

Steps run in order. Passthrough: `passthrough step=N` (default: last step); args after `--` on the CLI are appended to that step's argv. For how passthrough relates to overrides and target variants, see [Parameterization](parameterization.md).

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

## Suites

```bake
suite dev { build test deploy }
suite ci { build lint test }
```

`--list` is filtered by the active suite (dev, or ci when `CI=1` or `--ci`). Running `bake ci` runs all targets in the `ci` suite.

## Cwd, desc, tags

- **cwd** — working directory for the target (relative to Bakefile root).
- **desc** — one-line description for `--list` and help.
- **tags** — optional list of tags for grouping.
