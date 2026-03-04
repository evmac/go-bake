package config

// Target is a runnable unit with steps (ordered list) and optional deps, env, args, cwd, desc, tags.
type Target struct {
	Name string

	// Line and Column are 1-based source positions for error reporting (0 = unknown).
	Line, Column int

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

	// Inputs and Outputs are paths/globs (relative to root) for incremental build cache.
	// When set, the target is skipped if cache hit and outputs are up to date.
	Inputs  []string
	Outputs []string

	// When: run target only if condition holds. At most one of WhenEnv / WhenCmd is set.
	// WhenEnv: skip unless os.Getenv(WhenEnv) != "".
	// WhenCmd: skip unless running Argv exits 0 (e.g. when cmd ["test", "-f", "file"]).
	WhenEnv string   // env var name
	WhenCmd []string // argv for guard command
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
