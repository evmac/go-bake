# Installing git hooks with `bake install hooks`

**`bake install hooks`** writes a **pre-commit hook** (`.git/hooks/pre-commit`) that runs **`bake precommit`**. Hooks are only installed when you define a **suite named `precommit`** in your Bakefile; that suite lists the targets to run before each commit. The hook uses the same `bake` binary and runs in the repo root.

You can run **`bake install hooks`** by itself (requires an existing Bakefile with `suite precommit { ... }` and a git repo), or **`bake install`** to install hooks together with shims and a minimal Bakefile if needed.

## Requirements

- A **Bakefile** that defines **`suite precommit { ... }`** with at least one target. If there is no suite named `precommit`, `bake install hooks` exits with an error and does not write a hook.
- A **git repo** (a `.git` directory). If you run `bake install hooks` outside a git repo, bake exits with an error.

## Default suite: `precommit`

Bake treats the suite name **`precommit`** as a convention: when that suite is present, **`bake install hooks`** deploys a hook that runs **`bake precommit`** (i.e. runs all targets in that suite). There is no separate keyword or mark; you just define a normal suite with that name.

Example Bakefile:

```bake
target format { desc "format" steps { exec ["bake", "fmt", "-w"] } }
target lint   { desc "lint"   steps { exec ["bake", "lint", "--fix"] } }
target test   { deps build desc "test" steps { exec ["go", "test", "./..."] } }
target build  { desc "build" steps { exec ["go", "build", "./..."] } }

suite precommit { format lint test }
```

When you run **`bake install hooks`**, bake writes a script that runs **`bake precommit`**. On commit, that runs the `format`, `lint`, and `test` targets in order. If any fail, the commit is aborted. If you omit `suite precommit { ... }`, bake does not install a hook (explicit user-defined behavior only).

## Usage

1. Add **`suite precommit { target1 target2 ... }`** to your Bakefile with the targets you want to run before every commit (e.g. format, lint, test).
2. From the repo root, run:
   ```sh
   bake install hooks
   ```
   Or run **`bake install`** to install hooks along with shims and to create a minimal Bakefile if one doesn’t exist (the minimal Bakefile does not define `suite precommit`, so hooks are not installed until you add it).
3. The next time you (or anyone else) runs `git commit`, git will run `.git/hooks/pre-commit`, which runs **`bake precommit`**. If any target in the suite fails, the commit is aborted.

## What the hook does

The installed script:

1. Changes to the repository root (the directory that contains `.git`).
2. Runs **`bake precommit`** (the suite you defined).
3. Exits with that command’s status (failure aborts the commit).

The hook uses the full path to the `bake` binary that was used when you ran `bake install hooks`, so it doesn’t depend on `bake` being on `PATH` at commit time.

## Example

Bakefile:

```bake
target format { desc "format" steps { exec ["bake", "fmt", "-w"] } }
target lint   { desc "lint"   steps { exec ["bake", "lint", "--fix"] } }
target build  { desc "build" steps { exec ["go", "build", "./..."] } }
target test   { deps build desc "test" steps { exec ["go", "test", "./..."] } }

suite precommit { format lint test }
```

After **`bake install hooks`**, `.git/hooks/pre-commit` runs **`bake precommit`**, which runs `format`, then `lint`, then `test`. If any fail, the commit is blocked. You can still run **`bake format`** and **`bake lint`** manually; the hook ensures the precommit suite runs (and passes) before each commit.

## See also

- [Bakefile reference — Precommit (hooks)](bakefile-reference.md#precommit-hooks) for the `suite precommit` convention.
- [CLI reference](cli-reference.md) for **`bake install`** and **`bake install hooks`**.
- [Installing shims](install-shims.md) for **`bake install shims`**.
