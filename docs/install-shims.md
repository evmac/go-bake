# Installing shims with `bake install`

The `bake install` command generates **shims** (simple shell wrapper scripts) for each target and suite defined in your Bakefile. These shims are placed in a local directory: `.bake/bin/`. By adding this directory to your `$PATH`, you can run your build targets directly as shell commands, without needing to type `bake` each time.

For example, if your Bakefile defines these targets:

```bake
target build { exec ["go", "build", "-o", "bin/app", "./cmd/app"] }
target test  { exec ["go", "test", "./..."] }
```

After running:

```sh
bake install
```

You’ll get:

- `.bake/bin/build`
- `.bake/bin/test`

Now you can run `build` or `test` in your terminal, as long as `.bake/bin` is first on your `$PATH`.

## Usage

1. Run `bake install` in your project directory.
2. Add `.bake/bin` to your PATH. To do so temporarily in your shell, run:
   
   ```sh
   export PATH="$(pwd)/.bake/bin:$PATH"
   ```
   Add this line to your `.envrc` (if using [direnv](https://direnv.net/)), `.bashrc`, `.zshrc`, or equivalent, for automatic inclusion when you `cd` to your project directory.

3. Now you can invoke targets directly:

   ```sh
   build       # runs your 'build' target
   test        # runs your 'test' target
   ```

   You can also run suites, if defined (e.g. `.bake/bin/ci`).

## How shims work

Each shim is a small shell script that runs the corresponding target via the main `bake` executable. For example, the contents of `.bake/bin/build` might look like:

```sh
#!/bin/sh
exec "/absolute/path/to/bake" build "$@"
```

This passes any arguments you supply through to the underlying bake runner.

## Design notes

- **`bake install` and PATH** — Shims in `.bake/bin` share names with targets (e.g. `test`, `act`, `ci`). If that dir is first on PATH, steps that run those names would invoke the shim (recursion or wrong tool). The runner therefore strips the project’s `.bake/bin` from PATH when executing steps and when guards, so steps always see system binaries. `bake install` remains safe to use; only subprocess env is sanitized.

