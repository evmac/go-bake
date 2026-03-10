# Getting started

## Install

- **Binary:** `go install github.com/evmac/go-bake/cmd/bake@latest`
- **From source:** Clone the repo and run `go build -o bin/bake ./cmd/bake`

## First Bakefile

Create a file named `Bakefile` in your project root:

```bake
target build {
  steps { exec ["go", "build", "./..."] }
}

target test {
  deps build
  steps { exec ["go", "test", "./..."] }
}
```

Run `bake --list`, `bake build`, `bake test`. For long-running daemons, add a **target up** with **workflow { ... }** and **daemon name { ... }** inside it, then use **`bake up`** / **`bake down`**. See [Bakefile reference](bakefile-reference.md) and [CLI reference](cli-reference.md).
