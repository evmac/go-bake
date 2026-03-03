# Bake

A minimal Make replacement in Go: one binary, one Bakefile, explicit DAG, typed args, and repo-scoped commands.

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
target build {
  desc "build the binary"
  steps { exec ["go", "build", "-o", "bin/app", "./cmd/app"] }
}

target test {
  deps build
  steps { exec ["go", "test", "./..."] }
}

suite local { build test }
suite ci { build test }
```

Then:

- `bake --list` — list targets (filtered by suite)
- `bake build` — run the `build` target
- `bake test` — run `build` then `test`
- `bake ci` — run the ci suite (build, test)
- `bake install` — create `.bake/bin` shims so you can run `build` / `test` from PATH

## Docs

- [Getting started](docs/getting-started.md)
- [Bakefile reference](docs/bakefile-reference.md)
- [CLI reference](docs/cli-reference.md)
- [Grammar spec](docs/grammar-spec.md)
- [LLM reference](docs/llm-reference.md)
- [Future roadmap](docs/future.md)

## License

MIT
