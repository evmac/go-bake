# Bakefile reference

## Overview

A Bakefile is a custom DSL. Keywords: `target`, `deps`, `steps`, `exec`, `cmd`, `shell`, `env`, `args`, `dotenv`, `suite`, `cwd`, `desc`, `tags`, `passthrough`. Comments: `#` to end of line. Planned: `when` (guard) — see [Future roadmap](future.md).

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

**Single-line form** (only when the file has exactly one target and one step):

```bake
target build cmd go build ./...
```

## Steps

- **exec** — canonical: explicit argv list. `exec ["go", "test", "./..."]`
- **cmd** — sugar: space-separated tokens. `cmd go test ./...`
- **shell** — run a script with a shell. `shell bash "go test ./... | tee out"`

Steps run in order. Passthrough: `passthrough step=N` (default: last step); args after `--` on the CLI are appended to that step's argv.

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
suite local { build test deploy }
suite ci { build lint test }
```

`--list` is filtered by the active suite (local, or ci when `CI=1` or `--ci`). Running `bake ci` runs all targets in the `ci` suite.

## Cwd, desc, tags

- **cwd** — working directory for the target (relative to Bakefile root).
- **desc** — one-line description for `--list` and help.
- **tags** — optional list of tags for grouping.
