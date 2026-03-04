# Parameterization: passthrough, overrides, and variants

Three distinct ways to vary how a target runs. They solve different use cases and can be combined.

| Mechanism | Use case | Who supplies the value | Status |
|-----------|----------|-------------------------|--------|
| **Passthrough** | Ad-hoc: “this run, I want to add arbitrary args to the step.” | Caller at invoke time (after `--`) | Implemented |
| **Overrides** | One-off: “this run, use different env or declared args without editing the Bakefile.” | Caller via CLI flags (e.g. `--set env.FOO=bar`) | Planned |
| **Target variants** | Codified: “we have named ways we run this target; CI/suites should say ‘run test with cover’.” | Bakefile defines names and their argv/env | Planned |

---

## Passthrough

**Use case:** The same target, but for this invocation the caller appends raw argv to a step (e.g. `bake test -- -v -count=2`). No change to the Bakefile; fully ad-hoc.

**How:** In the Bakefile, `passthrough step = N`. On the CLI, args after `--` are appended to that step’s argv.

**Good for:** Debug flags, one-off options, passing through to the underlying tool without defining a new target.

**Limitation:** When running via a **suite**, no passthrough is passed (suite runs each target with no CLI args). So “run test with coverage” in CI either needs a separate target today or a variant later.

---

## Overrides

**Use case:** Override env vars or declared args for a single run from the CLI (e.g. `bake deploy --set env.ENV=prod --set args.region=eu`). Same target definition; different inputs for this run.

**How (planned):** CLI flag such as `--set env.FOO=bar` or `--set args.NAME=value`. Values apply to the run; they don’t change the Bakefile.

**Good for:** Switching env (prod/staging), toggling a declared arg, CI secrets or env without editing the Bakefile.

**Difference from passthrough:** Overrides set **named** env/args that the Bakefile already knows about. Passthrough is **raw argv** for a step; the Bakefile doesn’t name the pieces.

---

## Target variants

**Use case:** Named, Bakefile-defined ways to run a target so you can say `bake test` vs `bake test cover` without duplicating the target. Suites and CI can refer to the variant by name.

**How (planned):** A block on the target (e.g. `variant` or `opts`) listing names and the extra argv (or env) for each. Invocation: `bake <target> [variant]`.

**Good for:** Codifying “test”, “test with coverage”, “build debug” as named options on one target; keeps one definition, multiple entry points.

**Difference from passthrough:** Variants are **defined in the Bakefile** and **named**. Passthrough is whatever the caller types after `--`. Variants are for “we always run test these few ways”; passthrough is for “this time I’m adding whatever I want.”

**Difference from overrides:** Overrides set env/args by name for one run. Variants attach a **preset** (fixed argv or env) to a target and give it a name so the CLI can select it.

---

## Summary

- **Passthrough** — Caller appends raw argv this run. Implemented.
- **Overrides** — Caller sets env/args this run via `--set`. Planned.
- **Variants** — Bakefile defines named presets for a target; caller picks one (e.g. `bake test cover`). Planned.

See the [future roadmap](future.md) for where overrides and variants are scheduled.
