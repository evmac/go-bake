# CLI reference

## Usage

```text
bake [global-flags] [target | suite] [target-args] [--] [passthrough]
```

## Global flags

| Flag | Description |
|------|-------------|
| `--list` | List targets (filtered by active suite). |
| `--ci` | Use CI suite for `--list`. |
| `--dry-run` | Print commands and dependency order; do not run. |
| `--explain <target>` | Show dependency chain, args, and resolved steps for a target. |
| `--why <target>` | Explain why the target would run or be skipped (incremental build: changed inputs, missing outputs, cache hit). |
| `--debug` | Enable debug logging (or set `BAKE_DEBUG=1`). |

## Commands

- **bake** (no args) — run the default target (first target or one named `default`).
- **bake &lt;target&gt;** — run the target and its dependencies.
- **bake &lt;suite&gt;** — run all targets in the suite (e.g. `bake ci`).
- **bake install** — create `.bake/bin` with shims for each target and suite; add that directory to PATH to run `build`, `test`, etc. without the `bake` prefix.

## Target args

Pass args after the target name:

- `--key value` or `-short value` — declared args (see Bakefile `args`) and live args (undeclared).
- `--` — everything after is passthrough (appended to the step selected by `passthrough step=N`).

Example: `bake deploy --env prod --dry_run true -- --verbose`

## Exit codes

- `0` — success
- `1` — target or step failed
- `2` — config/usage error (e.g. no Bakefile, unknown target)
