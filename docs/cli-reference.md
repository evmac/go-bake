# CLI reference

## Usage

```text
bake [global-flags] [target [preset] | suite] [target-args] [--] [passthrough]
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
| `--json` | Emit NDJSON event stream to stdout (run_start, target_start/end, step_start/end, cache_skip, when_skip, run_end); step output goes to stderr. |
| `--max-parallel <n>` | Max targets to run at once (by DAG level); default 1 (sequential). |
| `--timing` | Print per-target duration summary to stderr after run (CI). |
| `--artifacts` | Print output paths (target path) to stderr after run (CI). |
| `-w` | Write result to file instead of stdout (only for `format` / `fmt`). |
| `--check` | Exit 1 if Bakefile would be changed (only for `format` / `fmt`); use in CI to enforce formatting. |
| `--watch` | Re-run target when inputs change (poll-based); requires a target name. |
| `--debug` | Enable debug logging (or set `BAKE_DEBUG=1`). |

## Environment (load-time)

| Variable | Effect |
|----------|--------|
| `BAKE_NO_AUTOFORMAT` | Skip automatic format-on-load (set to `1` or `true` to disable). |
| `BAKE_NO_AUTOLINT` | Skip automatic lint-on-load (set to `1` or `true` to disable). |

## Environment (containerized targets)

| Variable | Effect |
|----------|--------|
| `BAKE_PULL` | When a target has **image** and runs in Docker: `always`, `never`, or `if-not-present` (default). |

## Commands

- **bake** (no args) — run the default target (first target or one named `default`).
- **bake &lt;target&gt;** — run the target and its dependencies.
- **bake &lt;target&gt; &lt;preset&gt;** — run the target with the named preset (e.g. `bake test cover`).
- **bake &lt;suite&gt;** — run all targets in the suite (e.g. `bake ci`).
- **bake format** / **bake fmt** — format the Bakefile (canonical indentation and clause order). Prints to stdout unless `-w` is set, which overwrites the Bakefile in place. Use **`--check`** to exit 1 if the file would change (e.g. in CI). Comments are not preserved. When bake loads a Bakefile (for any target/suite run), it automatically runs format `-w` unless **`BAKE_NO_AUTOFORMAT`** is set (e.g. `1` or `true`).
- **bake lint** — run the Bake linter; applies fixable changes when **`--fix`** is set and reports only unfixable findings. Use **`--config <path>`** for a config file (default: `.bake/config`); **`--json`** for NDJSON; **`--disable <rule-id>`** to disable a rule and persist to config. When bake loads a Bakefile, it automatically runs lint with fix unless **`BAKE_NO_AUTOLINT`** is set. See [Lint rules](lint.md).
- **bake install** — install all components: if no Bakefile exists, create a minimal one (in repo root if in a git repo, else current dir); then install shims and hooks. Use **`bake install shims`** to only create `.bake/bin` shims; **`bake install hooks`** to only write `.git/hooks/pre-commit` (requires a Bakefile and a git repo). Add `.bake/bin` to PATH to run targets without the `bake` prefix.
- **bake install shims** — create `.bake/bin` with a shim for each target and suite (requires a Bakefile).
- **bake install hooks** — write `.git/hooks/pre-commit` to run **`bake precommit`** (requires a Bakefile that defines **`suite precommit { ... }`** and a git repo). See [Installing git hooks](install-hooks.md).

## Target args

Pass args after the target name:

- `--key value` or `-short value` — declared args (see Bakefile `args`) and live args (undeclared).
- `--` — everything after is passthrough (appended to the step selected by `passthrough step=N`).

Example: `bake deploy --env prod --dry_run true -- --verbose`

## Exit codes

- `0` — success
- `1` — target or step failed; or `format` / `fmt --check` when Bakefile is not formatted; or **`lint`** when there are findings
- `2` — config/usage error (e.g. no Bakefile, unknown target)
