# Future roadmap

Deferred features and design notes. See the main plan for full context.

## v1.2

- **Conditionals (when)** — run a target only when a condition holds; skip (no-op) otherwise. Proposed form: `when env VAR` (truthy env) or `when cmd ["prog", "args"]` (exit 0 = run). Enables e.g. integration tests only when DB is up or when `RUN_INTEGRATION=1`. Dependencies are still resolved; guard is evaluated before running the target’s steps.
- **Profiles as overlays** — first-class profile overlays (base → profile → CLI).
- **Include/import** — multiple Bakefiles, namespaces.
- **Structured passthrough** — declare passthrough target per step; named channels.
- **Override via CLI** — `--set env.FOO=bar` (explicit flag).
- **Concurrency controls** — per-task pools, `--max-parallel`, mutex for shared resources.
- **CI ergonomics** — timing breakdown, artifact summaries.
- **Machine-readable output** — `--json` event stream.

## v2 / later

- **Agent / LLM API** — `bake plan --json`, capability flags, sandbox, `bake api schema/list/explain`.
- **Remote cache / artifacts** — content-hash cache, optional remote.
- **Make → Bake conversion** — script or mapping guide.
- **Windows first-class** — execution and testing on Windows.
- **Idempotency / caching for agents** — content-hash cache keyed by inputs + params.
- **Background resources** (deferred) — `background: true`, PID/container tracking, `bake down`. Follows conditionals; use `when` + deps for “ensure DB up then run tests” until then.

## Resolved

- **v1.1 incremental builds** — Implemented: `inputs` / `outputs` per target, content hashing, `.bake/cache/`, skip when up to date.
- **v1.1 --why &lt;task&gt;** — Implemented: lists changed inputs, missing outputs, or "up to date (cache hit)".
- **v1.1 structured config validation** — Implemented: duplicate target/suite, unknown deps, suite→target refs; errors include file:line:col.
- **Windows** — Parser and spec are portable; execution on Windows is not supported in v1. Design avoids POSIX-only assumptions so Windows can be added later.
- **Remote caching** — Not in v1; local-only.
