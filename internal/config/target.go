package config

// Target is a runnable unit with steps (ordered list) and optional deps, env, args, cwd, desc, tags.
type Target struct {
	Name string

	// Deps are DAG edges; execution runs these before this target.
	Deps []string

	// Steps are intra-node sequencing; each step = runner + argv (and optional cwd, env, timeout).
	Steps []Step

	// Env is per-target env overlay (key=value).
	Env map[string]string

	// Args: declared per-target (name, type, short, default); interpolated in steps as {{.argName}}.
	Args []ArgDecl

	// Cwd is the working directory for this target.
	Cwd string

	// Desc is a one-line description for --list and help.
	Desc string

	// Tags for grouping/search (optional).
	Tags []string

	// PassthroughStep is the 1-based step index that receives raw args after "--"; 0 = last step.
	PassthroughStep int
}

// Step is the argv-first semantic model: runner + argv (+ optional cwd, env, timeout, ok_exit_codes).
type Step struct {
	// Runner: "" for direct exec; "bash", "sh", "zsh", "fish" for shell -c.
	Runner string
	// Argv is the canonical form; no string parsing at run time.
	Argv []string
	// Cwd, Env, Timeout, OkExitCodes are optional per-step overrides.
	Cwd         string
	Env         map[string]string
	Timeout     string
	OkExitCodes []int
}

// ArgDecl declares a target argument (name, type, short flag, default, required/enum).
type ArgDecl struct {
	Name     string
	Type     string // "string", "bool", etc.
	Short    string
	Default  string
	Required bool
	Enum     []string
}
