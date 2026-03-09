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

	// Private: if true, target is hidden from --list (and --choose); still runnable by name or as a dep.
	Private bool

	// Tags for grouping/search (optional).
	Tags []string

	// PassthroughStep is the 1-based step index that receives raw args after "--"; 0 = last step.
	// Deprecated: use Passthrough slice; when Passthrough is non-empty it takes precedence.
	PassthroughStep int

	// Passthrough lists steps that receive CLI args after "--". When multiple, args are split by "--" in order.
	Passthrough []PassthroughSlot

	// Inputs and Outputs are paths/globs (relative to root) for incremental build cache.
	// When set, the target is skipped if cache hit and outputs are up to date.
	Inputs  []string
	Outputs []string

	// When: run target only if condition holds. At most one of WhenEnv / WhenCmd is set.
	// WhenEnv: skip unless os.Getenv(WhenEnv) != "".
	// WhenCmd: skip unless running Argv exits 0 (e.g. when cmd ["test", "-f", "file"]).
	WhenEnv string   // env var name
	WhenCmd []string // argv for guard command

	// Presets are named presets (extra argv and/or env) for this target; e.g. bake test cover.
	Presets []Preset

	// Pool limits concurrent execution of targets using the same pool name (semaphore per name).
	Pool string
	// Mutex ensures only one target holding this mutex name runs at a time (mutex per name).
	Mutex string
}

// PassthroughSlot is a step that receives passthrough args (by index or optional name).
type PassthroughSlot struct {
	Step int    // 1-based step index
	Name string // optional name for tooling; empty for default single-slot
}

// Preset is a named overlay for a target: optional desc, optional steps (replaces target steps when set), argv and/or env.
type Preset struct {
	Name  string            // preset name (e.g. "cover")
	Desc  string            // optional description (for --list)
	Steps []Step            // optional; when non-nil, use these steps instead of target steps when this preset is run
	Argv  []string          // extra argv for the passthrough step (ignored when Steps is set)
	Env   map[string]string // env overlay when this preset is selected
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

// PresetByName returns the preset with the given name, or nil.
func (t *Target) PresetByName(name string) *Preset {
	for i := range t.Presets {
		if t.Presets[i].Name == name {
			return &t.Presets[i]
		}
	}
	return nil
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
