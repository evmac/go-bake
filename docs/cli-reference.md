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
| `--no-daemon` | Run in-process only; do not start or connect to the **baked** daemon. |
| `--version` | Print version and exit. |
| `--debug` | Enable debug logging (or set `BAKE_DEBUG=1`). |

## Environment (load-time)

| Variable | Effect |
|----------|--------|
| `BAKE_NO_AUTOFORMAT` | Skip automatic format-on-load (set to `1` or `true` to disable). |
| `BAKE_NO_AUTOLINT` | Skip automatic lint-on-load (set to `1` or `true` to disable). |
| `BAKE_DAEMON` | When set to `0`, disable autostart and connecting to **baked** (run in-process only). |
| `BAKE_NO_DAEMON` | When set (e.g. `1`), same as `BAKE_DAEMON=0`; run in-process only. |

## Environment (containerized targets)

| Variable | Effect |
|----------|--------|
| `BAKE_PULL` | When a target has **image** and runs in Docker: `always`, `never`, or `if-not-present` (default). |
| `BAKE_RUNTIME` | Container runtime: `docker` (default), `podman`, `containerd`, or `crio`. Podman uses default socket when `DOCKER_HOST` unset. |

## Commands

- **bake** (no args) — run the default target (first target or one named `default`).
- **bake &lt;target&gt;** — run the target and its dependencies.
- **bake &lt;target&gt; &lt;preset&gt;** — run the target with the named preset (e.g. `bake test cover`).
- **bake &lt;suite&gt;** — run all targets in the suite (e.g. `bake ci`).
- **bake format** / **bake fmt** — format the Bakefile (canonical indentation and clause order). Prints to stdout unless `-w` is set, which overwrites the Bakefile in place. Use **`--check`** to exit 1 if the file would change (e.g. in CI). Comments are not preserved. When bake loads a Bakefile (for any target/suite run), it automatically runs format `-w` unless **`BAKE_NO_AUTOFORMAT`** is set (e.g. `1` or `true`).
- **bake lint** — run the Bake linter; applies fixable changes when **`--fix`** is set and reports only unfixable findings. Use **`--config <path>`** for a config file (default: `.bake/config`); **`--json`** for NDJSON; **`--disable <rule-id>`** to disable a rule and persist to config. When bake loads a Bakefile, it automatically runs lint with fix unless **`BAKE_NO_AUTOLINT`** is set. See [Lint rules](lint.md).
- **bake install** — install all components: if no Bakefile exists, create a minimal one (in repo root if in a git repo, else current dir); then install shims and hooks. Use **`bake install shims`** to only create `.bake/bin` shims; **`bake install hooks`** to only write `.git/hooks/pre-commit`; **`bake install daemon`** to register **baked** with systemd (Linux) or launchd (macOS). Add `.bake/bin` to PATH to run targets without the `bake` prefix.
- **bake install shims** — create `.bake/bin` with a shim for each target and suite (requires a Bakefile).
- **bake install hooks** — write `.git/hooks/pre-commit` to run **`bake precommit`** (requires a Bakefile that defines **`suite precommit { ... }`** and a git repo). See [Installing git hooks](install-hooks.md).
- **bake install daemon** — write a systemd user unit (Linux) or launchd plist (macOS) for **baked** in the current workspace, then enable/load and start. See [Install daemon](install-daemon.md).
- **bake up** — Run the **target named `up`** (if defined): execute its **workflow** (targets in order with deps, then start each daemon defined in that target). State is written to `.bake/state.json`.
- **bake down** — Stop all daemons recorded in `.bake/state.json` (from a previous **`bake up`**).
- **bake down** *daemon* — Stop only the named daemon.

## Baked daemon

By default, when you run **bake** (e.g. `bake build` or `bake up`), bake tries to use the **baked** daemon: it starts **baked** automatically if it is not already running, then sends the request over a Unix socket (`.bake/baked.sock`). The daemon watches the Bakefile (and imports), reloads config on change, and updates `.bake/bin` shims so they stay in sync without running **bake install shims**.

- **Manual start:** Run **`baked`** from the repo root to start the daemon in the foreground. Use **`baked --no-watch`** to disable file watching (e.g. for tests). Use **`baked --tcp localhost:9876`** to also listen on TCP. Use **`baked --workspace /other/repo`** (repeatable) to manage additional workspaces from a single daemon instance.
- **Opt-out:** Use **`--no-daemon`** for that run, or set **`BAKE_DAEMON=0`** or **`BAKE_NO_DAEMON=1`** to run in-process only (no autostart, no connect). CLI flag overrides env.
- **One daemon per workspace:** Baked runs until stopped (SIGTERM / interrupt). One process per directory tree that contains a Bakefile (or multiple workspaces with `--workspace`).
- **Signals:** SIGUSR1 forces config reload across all workspaces. SIGUSR2 dumps daemon status to the log. SIGINT/SIGTERM/SIGHUP shuts down.
- **Persistent queue:** Incoming requests are persisted to `.bake/queue.json` before execution; on restart, pending items are replayed.
- **Health checker:** Every 30s, baked validates all daemon PIDs/containers and removes dead entries. Detects system sleep (gap > 90s) for full re-check.
- **Hot reload:** When the Bakefile changes, baked diffs the `up` target daemons and automatically starts/stops/restarts changed daemons.

For registering **baked** with systemd or launchd (start on login/boot), see [Install daemon](install-daemon.md).

## Target args

Pass args after the target name:

- `--key value` or `-short value` — declared args (see Bakefile `args`) and live args (undeclared).
- `--` — everything after is passthrough (appended to the step selected by `passthrough step=N`).

Example: `bake deploy --env prod --dry_run true -- --verbose`

## Exit codes

- `0` — success
- `1` — target or step failed; or `format` / `fmt --check` when Bakefile is not formatted; or **`lint`** when there are findings
- `2` — config/usage error (e.g. no Bakefile, unknown target)
