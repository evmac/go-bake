# LLM reference (minimal Bakefile grammar)

Use this for reliable Bakefile generation.

## Structure

- **dotenv** — `dotenv .env .env.local`
- **suite** — `suite name { target1 target2 target.preset }`
- **target** — `target name { ... }` or single-line `target name cmd prog arg1 arg2`

## Target body (bracketed)

- **deps** — `deps dep1, dep2`
- **steps** — `steps { exec ["prog","arg1"] }` or `cmd prog arg1` or `shell bash "script"`
- **env** — `env { KEY value }`
- **args** — `args name type short default`
- **cwd** — `cwd path`
- **desc** — `desc "one line"`
- **tags** — `tags t1 t2`
- **passthrough** — `passthrough step = 1`
- **preset** — `preset name { desc "..." argv ["flag"] env { K V } steps { exec [...] } }` — named preset (argv appends, env overlays, steps replaces)
- **image** — `image "alpine:3.19"` or `image alpine` — run steps in a container (Docker); requires Docker.
- **unsafe** — run on host even when **image** is set.
- **net** / **vol** — `net mynet`, `vol myvol`, or `vol data ./path` — attach container to named network/volume (first reference creates).

## Step forms

- `exec ["go","build","./..."]` — canonical argv
- `cmd go build ./...` — tokenized
- `shell bash "go test | tee out"` — shell -c script

## Interpolation

- `{{.argName}}` — declared arg
- `{{.live.key}}` — live (undeclared) arg

## CLI (relevant for generation)

- **bake lint** — Lint Bakefile; `--fix` to auto-fix (prefer `exec` over `cmd`, brackets). Prefer `exec` and `desc` for targets when generating.
- **bake --watch &lt;target&gt;** — Re-run when inputs change.

## Example

```bake
dotenv .env
target build { desc "build binary" steps { exec ["go","build","./..."] } }
target test {
  desc "run tests"
  deps build
  steps { exec ["go","test","./..."] }
  preset cover {
    desc "run tests with coverage"
    argv ["-coverprofile=coverage.out"]
  }
}
suite dev { build test }
suite ci { build test.cover }
```
