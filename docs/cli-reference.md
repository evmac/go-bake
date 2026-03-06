# CLI reference

## Usage

```text
bake [global-flags] [target | suite] [target-args] [--] [passthrough]
```

## Global flags

| Flag | Description |
|------|-------------|
| `--list` | List targets (filtered by active suite). |
| `--status` | With `--list`, show per-target incremental status (would run / skipped). |
| `--ci` | Use CI suite for `--list`. |
| `--dry-run` | Print commands and dependency order; do not run. |
| `--show-cmd` | Print each command to stderr before running (or set `BAKE_SHOW_CMD=1`). |
| `--set key=value` | Override env or args (e.g. `--set env.FOO=bar`, `--set args.NAME=value`); repeatable. |
| `--profile <name>` | Use named profile (env/dotenv overlay); or set `BAKE_PROFILE`. |
| `--choose` | Interactive menu to pick a target; without TTY prints one target per line (pipe to fzf). |
| `--graph` [=dot\|json] | Emit dependency graph (DOT or JSON); optional positional arg is target for subgraph. |
| `--what-depends-on <target>` | List targets that depend on the given target. |
| `--explain <target>` | Show dependency chain, args, and resolved steps for a target. |
| `--why <target>` | Explain why the target would run or be skipped (incremental build: changed inputs, missing outputs, cache hit). |
| `-w` | Write result to file instead of stdout (only for `format` / `fmt`). |
| `--check` | Exit 1 if Bakefile would be changed (only for `format` / `fmt`); use in CI to enforce formatting. |
| `--debug` | Enable debug logging (or set `BAKE_DEBUG=1`). |

## Commands

- **bake** (no args) — run the default target (first target or one named `default`).
- **bake &lt;target&gt;** — run the target and its dependencies.
- **bake &lt;suite&gt;** — run all targets in the suite (e.g. `bake ci`).
- **bake format** / **bake fmt** — format the Bakefile (canonical indentation and clause order). Prints to stdout unless `-w` is set, which overwrites the Bakefile in place. Use **`--check`** to exit 1 if the file would change (e.g. in CI). Comments are not preserved.
- **bake install** — create `.bake/bin` with shims for each target and suite; add that directory to PATH to run `build`, `test`, etc. without the `bake` prefix.

## Target args

Pass args after the target name:

- `--key value` or `-short value` — declared args (see Bakefile `args`) and live args (undeclared).
- `--` — everything after is passthrough (appended to the step selected by `passthrough step=N`).

Example: `bake deploy --env prod --dry_run true -- --verbose`

## Exit codes

- `0` — success
- `1` — target or step failed; or `format` / `fmt --check` when Bakefile is not formatted
- `2` — config/usage error (e.g. no Bakefile, unknown target)
