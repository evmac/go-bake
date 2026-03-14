# Future roadmap

Deferred features and design notes. **Target everything for v1 when possible.** Only increment major version (to v2) when we actually break previously working functionality; until then, ship as v1.x.

See the container/compose plan for architecture (containers, daemons, lifecycle, agentic).

---

## v1.0 — Initial release (done)

- [x] **CLI** — `bake [target | suite]`, default target, `--list` / `--ci` (suite-filtered list), `--dry-run`, `--explain <target>`, `bake install` (shims in `.bake/bin`).
- [x] **Bakefile DSL** — Targets and suites; `deps`, `steps` (`exec` / `cmd` / `shell`), `env`, `args`, `cwd`, `desc`, `dotenv`; declared and live args; template expansion `{{.name}}` / `{{.live.key}}`. Comments: `#` to end of line (lexer elides; parser ignores).
- [x] **Basic passthrough** — `passthrough step = N`; args after `--` on CLI are appended to that step's argv.

## v1.1 — Incremental builds and validation (done)

- [x] **Incremental builds** — `inputs` / `outputs` per target, content hashing, `.bake/cache/`, skip when up to date.
- [x] **--why &lt;target&gt;** — Explain why a target would run or be skipped (changed inputs, missing outputs, or cache hit).
- [x] **Structured config validation** — Duplicate target/suite, unknown deps, invalid suite→target refs; errors with file:line:col.

## v1.2 — Import and conditionals (done)

- [x] **Include/import** — Import statement (e.g. `import "./ops.bake"` or `import "ops.bake"`).
- [x] **Execution ordering resolution** — Ordering well-defined across multiple files (import order; duplicate target/suite names produce validation errors; cycle detection).
- [x] **Conditionals (when)** — Run a target only when a condition holds; skip (no-op) otherwise. Form: `when env VAR` or `when cmd ["prog", "args"]`. Dependencies still resolved; guard evaluated before target steps.
- [x] **Test coverage** — tests updated to improve coverage and integrate with CI.

## v1.3 — Profiles, CLI, formatter, and discoverability (done)

- [x] **Profiles as overlays** — First-class profile overlays (base → profile → CLI).
- [x] **Override via CLI** — `--set env.FOO=bar` (explicit flag). Use case: one-off env/args for a run without editing the Bakefile. See [Parameterization](parameterization.md) (passthrough vs overrides vs presets).
- [x] **Ordering and formatter** — The order stateful entities appear is execution order. Formatter (add or extend) auto-orders entities; **`--check`** verifies Bakefiles are ordered. Checks run before integration (e.g. before baked).
- [x] **Interactive picker** — `--choose`: menu to select a target; when not a TTY, emit list (one per line) so it can be piped to fzf or similar.
- [x] **Private targets** — Targets hidden from `--list` (per-target or file-level `private` at top of Bakefile); only callable as deps or by explicit name.
- [x] **Dependency graph** — `--graph` [format]: emit target dependency graph (e.g. DOT or JSON) for visualization or tooling.
- [x] **Reverse deps** — `--what-depends-on <target>`: list targets that depend on the given target (refactoring and impact analysis).
- [x] **List with status** — Optional `--list` output showing per-target incremental status (would run / skipped) when useful.

## v1.4 — Passthrough, presets, concurrency, watch, CI, and JSON output (done)

Normalized use cases: [Parameterization](parameterization.md) (passthrough · overrides · presets).

- [x] **Structured passthrough** — Declare passthrough per step; named channels; split args by `--`.
- [x] **Target preset block** — Define named presets on a target (e.g. `bake test` vs `bake test cover`); codified in Bakefile. See [Parameterization](parameterization.md).
- [x] **Concurrency controls** — Per-task pools, `--max-parallel`, mutex for shared resources.
- [x] **CI ergonomics** — Timing breakdown (`--timing`), artifact summaries (`--artifacts`).
- [x] **Watch mode** — `bake --watch <target>`: re-run target when inputs change (poll-based).
- [x] **Machine-readable output** — `--json` event stream (NDJSON).
- [x] **Bake linter** — `bake lint` with rules (prefer-exec, brackets-only, require-desc), `--fix`, config via `.bake-lint` or `.bake-lint.yaml`. See [Lint](lint.md).
- [x] **Precommit and install-hooks** — Define **`suite precommit { ... }`**; **`bake install hooks`** writes `.git/hooks/pre-commit` to run **`bake precommit`** (optional; only when that suite is present). Auto format and lint on load (unless `BAKE_NO_AUTOFORMAT` / `BAKE_NO_AUTOLINT`); lint applies fixes and surfaces only unfixable errors.
- [x] **Default suite ci** — **`suite ci`** is the conventional default for CI: **`--ci`** / **`CI=1`** and **`bake ci`** use it. Define **`suite ci { build lint test }`** and run **`bake ci`** in your CI config. Possible future: **`bake install ci`** to emit a minimal CI workflow snippet that runs **`bake ci`** (see backlog).

## v1.5 — Containerization support (done)

- [x] **Base image + container executor** — Image statement; run steps in container when image present. Docker as default runtime; **`BAKE_PULL`** (always / never / if-not-present) for download semantics. Honor **`unsafe`**, opts out of sandbox.
- [x] **Volumes/networks runtime** — First reference to a named **`net`** or **`vol`** (in execution order) creates it; later targets attach to the same. No separate network/volume block keywords.

## v1.6 — Runtimes, daemons, and lifecycle (done)

- [x] **Runtime selection** — User config to choose runtime (Podman, containerd, CRI-O); OCI internally. **`BAKE_RUNTIME`** (docker | podman | containerd | crio); Podman uses default socket when **`DOCKER_HOST`** unset.
- [x] **Composable sub-blocks** — Workflow and daemons are not first-class; they are **sub-blocks inside a target**. **`bake up`** runs the **target named `up`**; that target has **workflow { ... }** (order of targets and daemons) and **daemon name { ... }** (definitions). **Note:** In targets you can omit the `steps { }` wrapper when there is only one step — use bare `exec ["..."]` or `cmd ...` (see [Bakefile reference](bakefile-reference.md)).
- [x] **Lifecycle** — State file (`.bake/state.json`), startup order (deps → workflow targets → daemons), teardown, **`bake up`** / **`bake down`** / **`bake down <daemon>`**.
- [x] **Schedule inside workflow** — Optional **schedule** inside the workflow block (e.g. `workflow { build schedule interval "1h" }` or `schedule cron "0 * * * *"`). Run workflow once (targets + daemons), then re-run workflow targets on cron or interval until interrupt; daemons stay up.
- [x] **Daemon container scope** — When a daemon has **image**, start it as a container (track container ID in state); **bake down** stops/removes the container. Host daemons (exec/cmd, no image) keep current PID-based lifecycle. Daemon blocks support **net** / **vol** like targets.
- [x] **--version** — **`bake --version`** prints the release version; set at build time via `-ldflags "-X main.Version=..."` (e.g. in the Homebrew formula).

## v1.7 — baked (background daemon) (done)

- [x] **baked** — Background daemon: target lifecycles, containers, error/recovery, caching, shim updates. Run-once semantics; watches Bakefile(s), integrates on disk updates. Queueing and prioritization; Bakefile format checks. Big lift.
- [x] **baked: persistent queue** — Persist run queue across baked restarts so in-flight or queued work can be resumed or replayed. Queue file at `.bake/queue.json`; at-least-once replay on startup.
- [x] **baked: remote / TCP listener** — `--tcp <addr>` flag: listen on TCP in addition to Unix socket for remote or containerized clients. Both listeners share the same serve loop.
- [x] **baked: multiple workspaces** — `--workspace <dir>` (repeatable): single baked instance managing multiple workspace roots. Each workspace has its own config, watcher, and queue. Requests include optional `workspace` field to target a specific workspace.
- [x] **baked: hot reload workflow** — On Bakefile change, diff the old and new `up` target daemons; stop removed, start added, restart changed daemons automatically without `bake down` / `bake up`.
- [x] **baked: sleep/wake and lifecycle** — Periodic health checker (30s interval) validates daemon PIDs/containers; removes stale entries. Detects long sleep (3× interval gap) and runs full check on wake.
- [x] **baked: SIGUSR1 / SIGUSR2** — SIGUSR1 forces config reload across all workspaces; SIGUSR2 dumps status to log. Protocol also supports `reload` and `status` request fields.

## v1.8 — Namespacing and import scope

- [ ] **Import namespacing** — Prefix or namespace for targets, suites, and profiles from imported Bakefiles (e.g. qualified reference so imports don't pollute the global name space).
- [ ] **Import scope / visibility** — Limit each Bakefile so it only has access to commands (targets, suites, profiles) defined in that file and in all Bakefiles it imports (transitively). A file cannot reference items defined in files that import it ("above" the current file). Entry point (main Bakefile) sees itself plus its import tree; an imported file's references are validated against itself plus its own imports only.

## v1.9 — Agent block and agentic

- [ ] **agent block** — Describes how bake should behave when an external driver (AI, script, orchestrator) is in control. Declarative constraints and observability in the Bakefile; not "run this agent binary." Scope: allowlist, capability, schema (see design notes below).
- [ ] **Agentic** — `bake plan --json`, allowlist enforcement, one approval per target (or per-plan). Env: `BAKE_AGENTIC`, `BAKE_APPROVED` (or equivalent).

**Agentic design notes (for future evaluation / release):**

- **Interpretation:** The agent is the *caller* of bake (e.g. an AI tool that runs `bake build`). The agent block does not run an agent; it defines how bake behaves when such a driver is in control: what targets are allowed, what the driver can discover, and what requires approval.
- **Core semantics:** (1) **Allowlist** — When `BAKE_AGENTIC=1`, only targets listed in the workflow's agent allowlist may run. (2) **`bake plan --json`** — Machine-readable execution plan (order + allowlist) so the driver can discover what it's allowed to do without executing. (3) **Approval** — e.g. `BAKE_APPROVED=target1,target2`; bake only runs targets in that set when agentic, giving one-approval-per-target (or per-plan if we add approval scope).
- **Additional semantics (candidates for v1.9 or later):**
  - **Capability / intent level** — e.g. `read-only` (plan, explain, list only) vs `execute` (run allowlisted targets). Agentic mode can default to read-only; execution requires explicit capability or approval.
  - **Schema in plan** — `bake plan --json` includes per-target desc, args (name, type, default), inputs/outputs, tags so the driver knows what each target does without parsing the Bakefile.
  - **Approval scope** — Per-target (each in `BAKE_APPROVED`) vs per-plan (approve whole list once) vs per-level (approve by DAG level).
  - **Agent-facing instructions** — Optional `instructions` or per-target hint in the agent block, emitted in plan output for the driver to show or reason about.
  - **Audit trail** — Optional `BAKE_AGENT_AUDIT_LOG`: append NDJSON of request vs allowed/run for "what did the agent try and what did bake allow?"
  - **Safe-by-default** — When agentic and no execute capability (or no approval), bake never runs steps; only plan/explain/list.
  - **Max dep depth** — e.g. `agent { allowlist build test; max_deps 1 }` so the driver can't pull in long dependency chains beyond what the author intended.

---

## Later / backlog

- **bake install ci** — Emit a minimal CI workflow (e.g. GitHub Actions) that runs **`bake ci`**, so new repos can run **`bake install ci`** and paste the snippet into their CI config.
- **Plugin system** — Pushed down the roadmap; not in scope for early v1.x. We avoid design decisions that would preclude a plugin system later.
- **Remote cache / artifacts** — Content-hash cache, optional remote.
- **Make → Bake conversion** — Script or mapping guide.
- **Docker-Compose → Bake conversion** — Script or mapping guide.
- **Windows first-class** — Execution and testing on Windows.
- **Idempotency / caching for agents** — Content-hash cache keyed by inputs + params.
- **`--dump`** — Emit parsed/resolved config (targets, deps, env, args) without executing; complements `--explain` / `--dry-run` for tooling and AI.
- **Output control** — `--quiet` (only task names/errors) and/or `--verbose` (full env/argv) for step output; CI and script friendliness.
- **Target output as input** — Use another target’s stdout as input (env or stdin) to a step; toolchain flows without ad-hoc files.
- **Filter by scope** — Run only targets matching a path or tag (e.g. “targets touching `./app/`” or tag `frontend`); ad-hoc slice for large repos.
- **List with deps** — `--list --deps` (or similar): show each target’s deps next to it in the list.
- **when os / when arch** — Platform conditionals (e.g. `when os linux`, `when arch amd64`) in addition to `when env` / `when cmd`.
- **Composable variants** — Allow combining multiple named modifiers on a single target invocation (e.g. `bake test cover race`), unlike presets which select exactly one named configuration. Presets are static and mutually exclusive; composable variants would layer on top of each other.

--

## Workflow, daemon, schedule (design notes)

**Schedule** — Done in v1.6. Lives **inside** the workflow block: `workflow { build schedule interval "1h" }` or `schedule cron "0 * * * *"`. Re-runs workflow targets on cron or interval; daemons stay up. No interaction with daemons (you don't schedule a daemon).

**Job** — Deprecated; not reserved. Single-step workflow is sufficient.

**Workflow: intended vs optional “run target”**  
- **Intended:** Workflow is the sub-block that holds **steps, inputs, outputs** (and optionally schedule). Running the target runs that workflow (its steps), then starts any daemons. No “list of target names” by design.
- **Optional “workflow run target”:** If we allowed a workflow to run another target, it could look like one of these (not committed):

```bake
# Option A: explicit run clause
target up {
  workflow {
    run build
    steps { exec ["migrate", "up"] }
    run test
  }
  daemon api { ... }
}

# Option B: workflow targets = list of targets to run before this workflow’s steps
target up {
  workflow {
    targets build migrate
    steps { exec ["seed"] }
  }
  daemon api { ... }
}
```

So “workflow selects a target to run” would be an extra clause (e.g. **run** *target* or **targets** *name* ...) inside the workflow block; primary content remains **steps** (and inputs/outputs/schedule).

**Daemon lifecycle: host vs container**  
- **Host (current):** Daemon body uses **exec** / **cmd** / **shell** (no **image**). `StartDaemon` runs an `exec.Cmd` on the host, records **PID** in `.bake/state.json`. **bake down** sends SIGTERM (or Kill on Windows) to that PID. Lifecycle = process.
- **Container (contextualized):** When daemon has **image** (and optionally **net**, **vol**), lifecycle could be: start a **container** (e.g. `docker run`), record **container ID** in state, **bake down** stops/removes that container. So daemon is “contextualized to container scope” when `image` is set: same daemon block, but execution and teardown are container-based instead of host process. Today **image** on a daemon is not used by `internal/lifecycle/daemon.go` — only host process is implemented; container-scoped daemon would be a follow-up (start container for daemon’s step, track ID, stop on down).

--

## Homebrew distribution

- [x] **Homebrew personal tap** — Separate repo ([homebrew-bake](https://github.com/evmac/homebrew-bake)) with CI that builds and publishes the formula on each go-bake release; version aligned with go-bake. Trigger from go-bake release workflow via `repository_dispatch`.

--

## VS Code extension and release CI

- [ ] **Bake syntax extension** — TextMate grammar for Bakefiles (`.bake`, `Bakefile`): keywords, comments, strings, templates `{{.…}}`, punctuation. Extension source in go-bake under `editors/vscode-bake/`.
- [ ] **Extension repo and publish** — Separate repo (e.g. vscode-bake) with CI that builds and publishes the extension on each go-bake release; version aligned with go-bake. Trigger from go-bake release workflow via `repository_dispatch`.
