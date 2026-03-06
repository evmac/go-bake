# LLM reference (minimal Bakefile grammar)

Use this for reliable Bakefile generation.

## Structure

- **dotenv** — `dotenv .env .env.local`
- **suite** — `suite name { target1 target2 }`
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

## Step forms

- `exec ["go","build","./..."]` — canonical argv
- `cmd go build ./...` — tokenized
- `shell bash "go test | tee out"` — shell -c script

## Interpolation

- `{{.argName}}` — declared arg
- `{{.live.key}}` — live (undeclared) arg

## Example

```bake
dotenv .env
target build { steps { exec ["go","build","./..."] } }
target test { deps build steps { exec ["go","test","./..."] } }
suite dev { build test }
```
