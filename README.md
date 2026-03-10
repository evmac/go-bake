# Bake

A minimal Make replacement in Go.

## Install

**Homebrew** (after [adding the tap](docs/homebrew.md)):

```bash
brew tap evmac/bake && brew install bake
```

**Go**:

```bash
go install github.com/evmac/go-bake/cmd/bake@latest
```

Or build from source:

```bash
git clone https://github.com/evmac/go-bake && cd go-bake && go build -o bin/bake ./cmd/bake
```

## Quick start

Create a `Bakefile` in your project root:

```bake
profile prod {
  env {
    GOOS linux
    GOARCH amd64
  }
}

target deploy {
  deps build
  steps { exec ["deploy.sh", "{{.env}}", "{{.dry_run}}"] }
}

target build {
  desc "build the binary"
  steps { exec ["go", "build", "-o", "bin/app", "./cmd/app"] }
}

target format {
  desc "format code (e.g. go fmt)"
  steps { exec ["go", "fmt", "./..."] }
}

target test {
  deps build
  steps { exec ["go", "test", "./..."] }
}

target lint {
  desc "lint Bakefile"
  steps { exec ["bake", "lint"] }
}

suite local { build format lint test }
suite ci { build test }
```

Then:

- `bake --list` — list targets (filtered by suite)
- `bake --version` — print version and exit
- `bake build` — run the `build` target
- `bake test` — run `build` then `test`
- `bake ci` — run the ci suite (build, test)
- `bake lint` — lint the Bakefile (optional `--fix`); see [Lint](docs/lint.md)
- `bake --watch build` — re-run `build` when inputs change
- **Container targets** — use **`image "alpine:3.19"`** (and optional **`net`** / **`vol`**) in a target to run its steps in Docker. Set **`BAKE_RUNTIME`** to `docker` (default), `podman`, `containerd`, or `crio` to choose the runtime. See [Bakefile reference](docs/bakefile-reference.md#image-unsafe-net-and-vol-containerization).
- **Workflow and daemons** — define **`workflow { build redis }`** and **`daemon redis { steps { exec ["redis-server"] } }`**; run **`bake up`** to run the workflow and start daemons in the background, **`bake down`** to stop them. See [Bakefile reference](docs/bakefile-reference.md#workflow-and-daemons-v16).
- `bake install` — install all components (create minimal Bakefile if none; install shims and hooks). Use **`bake install shims`** for shims only, **`bake install hooks`** for the pre-commit hook only. See [Installing shims](docs/install-shims.md) and [Installing git hooks](docs/install-hooks.md).

## CI

The [CI workflow](.github/workflows/ci.yml) runs on push/PR. To run it locally with [act](https://github.com/nektos/act) (requires Docker and `brew install act`):

```bash
bake act
# or: act push
```

The project’s [.actrc](.actrc) pins the runner image so Go and the workflow run correctly.

## Docs

- [Book of Bake](docs/book-of-bake.md)
- [Getting started](docs/getting-started.md)
- [Bakefile reference](docs/bakefile-reference.md)
- [CLI reference](docs/cli-reference.md)
- [Parameterization (passthrough, overrides, presets)](docs/parameterization.md)
- [Installing shims](docs/install-shims.md)
- [Installing git hooks](docs/install-hooks.md)
- [Lint](docs/lint.md)
- [Testing and coverage](docs/testing-and-coverage.md)
- [Grammar spec](docs/grammar-spec.md)
- [LLM reference](docs/llm-reference.md)
- [Homebrew](docs/homebrew.md)
- [Future roadmap](docs/future.md)
- [Releasing](docs/releasing.md)
