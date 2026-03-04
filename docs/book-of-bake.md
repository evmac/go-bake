# Book of Bake

A short statement of bake's identity and principles — what we believe guides its design and messaging.

---

## 1. Bake manages the defined processes that govern your repo

Bake gives you a clearly defined place—the Bakefile—to declare all the automated workflows your project needs. In one file, you specify targets (like "build," "test," or "deploy"), state what each one depends on, and describe the steps required to achieve them. Instead of having logic scattered across hand-written scripts or hidden in CI configs, Bake brings these routines front and center, making your project's core automations visible, reliable, and consistent. When you run `bake`, you’re using a single, unified process that faithfully carries out the tasks your team has explicitly defined, ensuring "how we build, test, and run" is both transparent and repeatable for everyone.

## 2. Bake is an organizational build tool

Bake’s purpose is to help you organize and run the commands and workflows *within* your repository. It does not try to manage software packages, lock dependency versions, or coordinate installs across systems. Nor does it attempt to be a “universal automation engine,” meaning it’s not aiming to automate tasks across your entire system, multiple repositories, or different languages and environments at once. Bake focuses on orchestrating the build, test, and operational routines that are particular to your project—serving as an automation backbone only inside that context. Everything Bake does, it does in service of keeping your project’s automation clean, scoped, and understandable.

## 3. Bake is exclusively repo-scoped

All Bake workflows are strictly bound to the project where the Bakefile lives. Every declared target, rule, and step operates within the current repo and its environment—never spilling into global settings or referencing outside repositories by default. This local-first philosophy keeps your automations self-contained and predictable: when you run Bake, you’re always acting on this codebase, using only the recipes and dependencies defined in this project’s own Bakefile. There are no hidden global hooks or cross-repo surprises; everything is visible and controlled, right where you work.

## 4. Bake believes in unity and minimalism

Simplicity breeds reliability: one tool, one configuration, one clear path. Bake is designed around a singular interface and a cohesive config philosophy—no fragmentation, no ambiguity. It avoids complexity for its own sake, resisting the temptation to multiply entry points or sprawl across concerns. Bake holds that minimalism isn’t just efficiency, but a trust: if you know how to run Bake, you know your build. The focus is on portability and predictability; the surface area stays small, putting clarity and confidence above endless extensibility.

## 5. Bake embraces visible, intentional structure

Bake's design philosophy is to make all dependencies and execution order transparent. Rather than relying on convention, implicit rules, or magic behaviors, everything flows from direct declarations in the Bakefile. The builds you define are the builds you get—no surprises or hidden chains.

## 6. Bake values clarity over cleverness

Bake champions explicitness at every level: arguments to commands, dependencies, inputs, and outputs are all stated outright. Convention can streamline things, but explicit, intentional design takes priority—ensuring workflows are understandable, reproducible, and easy to audit.

## 7. Bake embodies extensibility, composability, and provability

Bake is rooted in the belief that powerful systems arise from structures that can grow, combine, and be interrogated with confidence. Extensibility means that growth is never blocked—your workflows can evolve without friction. Composability insists that workflows should be built up from understandable pieces, connected transparently. Provability affirms that every action, dependency, and outcome should be open to inspection and explanation. Together, these principles ensure Bake empowers you to build something greater than the sum of its parts—always with clarity and trust.

## 8. Bake supports people and automation

Bake makes workflows discoverable, explainable, and predictable—for humans and machines alike. Its philosophy is to remove surprises: everything is declared, everything can be surfaced, and nothing is hidden behind convention or “magic.” Bake empowers developers to explore, reason, and audit what will run, fostering trust and transparency.

Whether through interactive commands or structured output, Bake encourages both human understanding and tool interoperability. Enabling AI- and agent-based workflows isn’t a bolt-on feature; it’s a natural extension of Bake’s explicitness and deterministic design. If a machine can understand your build process, so can you — and vice versa.

## 9. Bake is local-first

Bake's design ensures that every workflow is runnable offline, without requiring a remote server or cloud integration. Your Bakefile and all its targets remain fully operational on your own machine, unaffected by network outages or external service disruptions. This empowers developers to iterate rapidly and confidently, anywhere—whether on a plane, in a secure environment, or during infrastructure downtime.

## 10. Bake is CI-native

Bake is engineered with continuous integration (CI) and automation at its core. Execution is fully deterministic, ensuring consistent build and test results every run. Incremental builds leverage robust content hashing to only rerun steps impacted by changes, maximizing efficiency and reliability. "But it works on my machine" should never be heard again.

---

See also: [Bakefile reference](bakefile-reference.md), [CLI reference](cli-reference.md), [Getting started](getting-started.md).
