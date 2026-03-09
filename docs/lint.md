# Bake linter

`bake lint` runs configurable rules over your Bakefile and reports findings (file:line). Use `--fix` to apply auto-fixes where possible.

## Usage

```text
bake lint [--fix] [--config <path>] [--json] [--disable <rule-id>]
```

- **`--fix`** — Apply auto-fixes (e.g. convert `cmd` to `exec`, quote exec elements, normalize to bracketed form). Writes the Bakefile in place.
- **`--config <path>`** — Config file path. Default: `.bake/config` in the Bakefile directory (falls back to `.bake-lint.yaml` or `.bake-lint`). If the file does not exist, all built-in rules are enabled at `warn`.
- **`--json`** — Emit one JSON object per finding (NDJSON) to stderr.
- **`--disable <rule-id>`** — Disable a rule by ID and persist the change to the config file. Does not run the linter.

Exit code: **0** if no findings; **1** if any findings; **2** on config/load error.

## Config file

Config lives at `.bake/config` (created by `--disable` or manually). Plain text, one rule per line:

```text
# bake lint config
# rule_id [warn|error|disabled]
prefer-exec warn
brackets-only warn
require-desc disabled
prefer-quoted-exec error
```

- Lines starting with `#` and empty lines are ignored.
- Each line: `rule_id [warn|error|disabled]`.
- **`disabled`** turns the rule off entirely.
- Rules not listed default to `warn` (enabled).
- Legacy config paths `.bake-lint.yaml` and `.bake-lint` are still supported (`.bake/config` takes precedence).

### Disabling rules

**Via CLI** (persists to `.bake/config`):

```bash
bake lint --disable require-desc
```

**Via config file** (edit `.bake/config`):

```text
require-desc disabled
```

## Built-in rules

| ID | Description | Fixable | Default |
|----|-------------|---------|---------|
| **prefer-exec** | Prefer `exec ["..."]` over `cmd ...` (single-line target or step). | Yes — converts to exec. | warn |
| **brackets-only** | Prefer bracketed form: `target name { ... }` and `steps { }` wrapper. Flags single-line targets and bare steps. | Yes — run formatter. | warn |
| **require-desc** | Warn when a target has no `desc`. | No | warn |
| **prefer-quoted-exec** | Prefer quoted strings in `exec []`, `inputs []`, `outputs []` (e.g. `exec ["go", "build"]` not `exec [go, build]`). | Yes — run `bake format -w`. | warn |

### prefer-exec

Flags use of `cmd` (space-separated tokens) and suggests `exec` (explicit argv list). Single-line targets (`target build cmd go build .`) are also flagged.

**Before:** `target build cmd go build .`
**After:** `target build { steps { exec ["go", "build", "."] } }`

### brackets-only

Flags single-line targets and bare steps (without `steps { }` wrapper). The formatter normalizes both.

### require-desc

Flags targets without a `desc` clause. Not auto-fixable — you need to add a description manually.

### prefer-quoted-exec

Flags unquoted elements in `exec []`, `inputs []`, and `outputs []` brackets. The formatter always quotes these elements, so running `bake format -w` (or relying on auto-format) fixes this automatically.

**Before:** `exec [go, build, "-o", bin/app, ./cmd/app]`
**After:** `exec ["go", "build", "-o", "bin/app", "./cmd/app"]`

## Auto-lint

When bake loads a Bakefile (for any target/suite run), it automatically runs `bake lint --fix` unless **`BAKE_NO_AUTOLINT`** is set. This applies fixes silently and only surfaces unfixable findings.

## Output

Human-readable (default): one line per finding:

```text
Bakefile:12: prefer exec over cmd for step build
Bakefile:20: target test has no desc
```

With **`--json`**, each line is a JSON object:

```json
{"file":"Bakefile","line":12,"column":0,"message":"prefer exec over cmd for step build","rule":"prefer-exec","fixable":true}
```

## CI

Run in CI to fail on lint findings:

```text
bake lint
```

To auto-fix and then check (e.g. in a pre-commit or format-and-check job):

```text
bake lint --fix
bake fmt --check
```
