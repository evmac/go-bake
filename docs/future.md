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

## v1.3 — Profiles, CLI, and formatter

- [ ] **Profiles as overlays** — First-class profile overlays (base → profile → CLI).
- [ ] **Override via CLI** — `--set env.FOO=bar` (explicit flag). Use case: one-off env/args for a run without editing the Bakefile. See [Parameterization](parameterization.md) (passthrough vs overrides vs variants).
- [ ] **Ordering and formatter** — The order stateful entities appear is execution order. Formatter (add or extend) auto-orders entities; **`--check`** verifies Bakefiles are ordered. Checks run before integration (e.g. before baked).
- [ ] **CI ergonomics** — Timing breakdown, artifact summaries.

## v1.4 — Passthrough, variants, concurrency, CI, and JSON output

Normalized use cases: [Parameterization](parameterization.md) (passthrough · overrides · variants).

- [ ] **Structured passthrough** — Declare passthrough target per step; named channels.
- [ ] **Target variant block** — Define named variants on a target (e.g. `bake test` vs `bake test cover`); codified in Bakefile, no separate target. See [Parameterization](parameterization.md).
- [ ] **Concurrency controls** — Per-task pools, `--max-parallel`, mutex for shared resources.
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

## v1.9 — VS Code extension and release CI

- [ ] **Bake syntax extension** — TextMate grammar for Bakefiles (`.bake`, `Bakefile`): keywords, comments, strings, templates `{{.…}}`, punctuation. Extension source in go-bake under `editors/vscode-bake/`.
- [ ] **Extension repo and publish** — Separate repo (e.g. vscode-bake) with CI that builds and publishes the extension on each go-bake release; version aligned with go-bake. Trigger from go-bake release workflow via `repository_dispatch`.

---

## Later / backlog

- **Import namespacing** — Prefix or namespace targets/suites from imported Bakefiles; deferred to a future release.
- **Plugin system** — Pushed down the roadmap; not in scope for early v1.x. We avoid design decisions that would preclude a plugin system later.
- **Remote cache / artifacts** — Content-hash cache, optional remote.
- **Make → Bake conversion** — Script or mapping guide.
- **Docker-Compose → Bake conversion** — Script or mapping guide.
- **Windows first-class** — Execution and testing on Windows.
- **Idempotency / caching for agents** — Content-hash cache keyed by inputs + params.

