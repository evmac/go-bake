# Future roadmap

Deferred features and design notes. See the main plan for full context.

## v1.1 (first after v1)

- **Incremental builds** — file inputs/outputs per task, mtime + content hashing, persistent build cache.
- **--why &lt;task&gt;** — list changed inputs / hash misses (depends on incremental build).
- **Structured config validation** — schema, line/col error messages.

## v1.2

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
- **Background resources** — `background: true`, PID/container tracking, `bake down`.
- **Make → Bake conversion** — script or mapping guide.
- **Windows first-class** — execution and testing on Windows.
- **Idempotency / caching for agents** — content-hash cache keyed by inputs + params.

## Resolved

- **Windows** — Parser and spec are portable; execution on Windows is not supported in v1. Design avoids POSIX-only assumptions so Windows can be added later.
- **Remote caching** — Not in v1; local-only.
