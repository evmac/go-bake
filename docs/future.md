# Future roadmap

Deferred features and design notes. **Target everything for v1 when possible.** Only increment major version (to v2) when we actually break previously working functionality; until then, ship as v1.x.

See the container/compose plan for architecture (containers, daemons, lifecycle, agentic).

---

## v1.0 — Initial release (done)

- [x] **CLI** — `bake [target | suite]`, default target, `--list` / `--ci` (suite-filtered list), `--dry-run`, `--explain <target>`, `bake install` (shims in `.bake/bin`).
- [x] **Bakefile DSL** — Targets and suites; `deps`, `steps` (`exec` / `cmd` / `shell`), `env`, `args`, `cwd`, `desc`, `dotenv`; declared and live args; template expansion `{{.name}}` / `{{.live.key}}`.
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

## v1.3 — Profiles, CLI, formatter, and discoverability

- [x] **Profiles as overlays** — First-class profile overlays (base → profile → CLI).
- [x] **Override via CLI** — `--set env.FOO=bar` (explicit flag). Use case: one-off env/args for a run without editing the Bakefile. See [Parameterization](parameterization.md) (passthrough vs overrides vs variants).
- [x] **Ordering and formatter** — The order stateful entities appear is execution order. Formatter (add or extend) auto-orders entities; **`--check`** verifies Bakefiles are ordered. Checks run before integration (e.g. before baked).
- [x] **Interactive picker** — `--choose`: menu to select a target; when not a TTY, emit list (one per line) so it can be piped to fzf or similar.
- [x] **Private targets** — Targets hidden from `--list` (per-target or file-level `private` at top of Bakefile); only callable as deps or by explicit name.
- [x] **Dependency graph** — `--graph` [format]: emit target dependency graph (e.g. DOT or JSON) for visualization or tooling.
- [x] **Reverse deps** — `--what-depends-on <target>`: list targets that depend on the given target (refactoring and impact analysis).
- [x] **List with status** — Optional `--list` output showing per-target incremental status (would run / skipped) when useful.

## v1.4 — Passthrough, variants, concurrency, watch, CI, and JSON output

Normalized use cases: [Parameterization](parameterization.md) (passthrough · overrides · variants).

- [ ] **Structured passthrough** — Declare passthrough target per step; named channels.
- [ ] **Target variant block** — Define named variants on a target (e.g. `bake test` vs `bake test cover`); codified in Bakefile, no separate target. See [Parameterization](parameterization.md).
- [ ] **Concurrency controls** — Per-task pools, `--max-parallel`, mutex for shared resources.
- [ ] **CI ergonomics** — Timing breakdown, artifact summaries.
- [ ] **Watch mode** — `bake --watch <target>`: re-run target (and deps as needed) when inputs change; dev ergonomics, local-first.
- [ ] **Machine-readable output** — `--json` event stream.

## v1.5 — Containerization support

- [ ] **Base image + container executor** — Image statement; StepExecutor; run steps in container when image present. Docker as default runtime; download semantics TBD. Honor `unsafe`, opts out of sandbox.
- [ ] **Volumes/networks runtime** — First reference to a named `net` or `vol` (in execution order) creates it; later targets attach to the same. No separate network/volume block keywords.

## v1.6 — Runtimes, daemons, and lifecycle

- [ ] **Runtime selection** — User config to choose runtime (Podman, containerd, CRI-O); OCI internally.
- [ ] **Composable sub-blocks** — Workflow block (0 or 1), daemon blocks (0+); schedule inside workflow. **Note:** In targets (and in workflow blocks when we add them), you can omit the `steps { }` wrapper when there is only one step — use bare `exec ["..."]` or `cmd ...` (see [Bakefile reference](bakefile-reference.md)).
- [ ] **Lifecycle** — State file, startup order (deps → workflow → daemons), teardown, `bake down` / `bake down <target>`.

## v1.7 — Agent block and agentic

- [ ] **agent block** — Inside workflow blocks; defines agentic semantics. Steps run before agent; all run on schedule. Scope to expand (allowlist, capability, schema).
- [ ] **Agentic** — `bake plan --json`, allowlist, one approval per target.

## v1.8 — baked (background daemon)

- [ ] **baked** — Background daemon: target lifecycles, containers, error/recovery, caching, shim updates. Run-once semantics; watches Bakefile(s), integrates on disk updates. Queueing and prioritization; Bakefile format checks. Big lift.

## v1.9 — Namespacing and import scope

- [ ] **Import namespacing** — Prefix or namespace for targets, suites, and profiles from imported Bakefiles (e.g. qualified reference so imports don’t pollute the global name space).
- [ ] **Import scope / visibility** — Limit each Bakefile so it only has access to commands (targets, suites, profiles) defined in that file and in all Bakefiles it imports (transitively). A file cannot reference items defined in files that import it (“above” the current file). Entry point (main Bakefile) sees itself plus its import tree; an imported file’s references are validated against itself plus its own imports only.

---

## Later / backlog

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

--

## VS Code extension and release CI

- [ ] **Bake syntax extension** — TextMate grammar for Bakefiles (`.bake`, `Bakefile`): keywords, comments, strings, templates `{{.…}}`, punctuation. Extension source in go-bake under `editors/vscode-bake/`.
- [ ] **Extension repo and publish** — Separate repo (e.g. vscode-bake) with CI that builds and publishes the extension on each go-bake release; version aligned with go-bake. Trigger from go-bake release workflow via `repository_dispatch`.
