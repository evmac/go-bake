package dsl

import (
	"strings"

	"github.com/alecthomas/participle/v2/lexer"
)

// AST types produced by the participle parser. Compiled to config.File by Compile().

// Bakefile is the root AST node.
type Bakefile struct {
	Dotenv  *DotenvLine  `@@?`
	Entries []*FileEntry `@@*`
}

// FileEntry is an import, Suite, Target, Profile, or file-level private marker.
// Workflow and daemons are not first-class; they are sub-blocks inside a target (e.g. target up { workflow { ... }; daemon x { ... } }).
type FileEntry struct {
	Import   *ImportLine    `  @@`
	Suite    *SuiteBlock    `| @@`
	Target   *TargetBlock   `| @@`
	Profile  *ProfileBlock  `| @@`
	Private  *PrivateMarker `| @@`
}

// DaemonBlock is "daemon" ident "{" body "}" — long-running unit (steps, env, cwd, image). Defined only inside a target (e.g. target up { workflow { ... }; daemon x { ... } }); not first-class.
type DaemonBlock struct {
	Pos  lexer.Position
	Name string      `"daemon" @Ident`
	Body *TargetBody `"{" @@ "}"`
}

// ProfileBlock is "profile" ident "{" dotenv? env? "}".
type ProfileBlock struct {
	Pos  lexer.Position
	Name string       `"profile" @Ident`
	Body *ProfileBody `"{" @@ "}"`
}

// ProfileBody contains dotenv and env entries.
type ProfileBody struct {
	Entries []*ProfileBodyEntry `@@*`
}

// ProfileBodyEntry is dotenv clause or env block.
type ProfileBodyEntry struct {
	Dotenv *ProfileDotenvClause `  @@`
	Env    *EnvBlock            `| @@`
}

// ProfileDotenvClause is "dotenv" followed by path strings.
type ProfileDotenvClause struct {
	Paths []ExecElem `"dotenv" ( @Ident | @String )*`
}

// PrivateMarker is "private" at file level; all targets in this file are private.
type PrivateMarker struct {
	Pos lexer.Position `"private"`
}

// ImportLine is "import" string (path to another Bakefile).
type ImportLine struct {
	Path QuotedString `"import" @String`
}

// DotenvLine is "dotenv" followed by file names.
type DotenvLine struct {
	Files []string `"dotenv" @Ident*`
}

// SuiteEntryToken captures a quoted string; unquotes on capture.
type SuiteEntryToken string

// Capture implements participle.Capture; unquotes if the value is a quoted string.
func (s *SuiteEntryToken) Capture(values []string) error {
	if len(values) == 0 {
		return nil
	}
	v := values[0]
	if len(v) >= 2 && v[0] == '"' && v[len(v)-1] == '"' {
		inner := v[1 : len(v)-1]
		var b []byte
		for i := 0; i < len(inner); i++ {
			if inner[i] == '\\' && i+1 < len(inner) && inner[i+1] == '"' {
				b = append(b, '"')
				i++
				continue
			}
			b = append(b, inner[i])
		}
		*s = SuiteEntryToken(string(b))
	} else {
		*s = SuiteEntryToken(v)
	}
	return nil
}

// SuiteEntryIdent is a single ident (e.g. build or test.cover); one token.
type SuiteEntryIdent struct {
	V string `@Ident`
}

// SuiteEntryQuoted is a quoted string; one token.
type SuiteEntryQuoted struct {
	V SuiteEntryToken `@String`
}

// SuiteEntry is either one ident or one quoted string so the parser consumes exactly one token per entry.
type SuiteEntry struct {
	Ident  *SuiteEntryIdent  `( @@ |`
	Quoted *SuiteEntryQuoted `  @@ )`
}

// Raw returns the entry as "target" or "target preset" (space-separated for config).
func (e *SuiteEntry) Raw() string {
	if e == nil {
		return ""
	}
	if e.Quoted != nil {
		return string(e.Quoted.V)
	}
	if e.Ident != nil {
		v := e.Ident.V
		// One dot => target.preset → "target preset"
		if strings.Count(v, ".") == 1 {
			return strings.Replace(v, ".", " ", 1)
		}
		return v
	}
	return ""
}

// SuiteBlock is "suite" ident "{" entry* "}". Each entry is ident (e.g. test.cover) or "quoted string".
type SuiteBlock struct {
	Pos     lexer.Position
	Name    string        `"suite" @Ident`
	Entries []*SuiteEntry `"{" ( @@ )* "}"`
}

// TargetBlock is "target" ident "{" ... "}" (bracketed) or "target" ident "cmd" ... (single-line).
type TargetBlock struct {
	Pos    lexer.Position
	Name   string      `"target" @Ident`
	Body   *TargetBody `( "{" @@ "}"`
	CmdTok []string    `  | "cmd" @Ident* )` // single-line: cmd token+
}

// TargetBody is the content inside target { }; each clause can appear in any order, args may repeat.
type TargetBody struct {
	Entries []*BodyEntry `@@*`
}

// BodyEntry is one of deps, steps, a single step (exec/cmd/shell), env, args, cwd, desc, tags, passthrough, inputs, outputs, when, private, preset, pool, mutex, image, unsafe, net, vol, workflow, daemon.
type BodyEntry struct {
	Deps        *DepsClause        `  @@`
	Steps       *StepsBlock        `| @@`
	Step        *Step              `| @@` // single step without "steps { }" wrapper
	Env         *EnvBlock          `| @@`
	Args        *ArgsClause        `| @@`
	Cwd         *CwdClause         `| @@`
	Desc        *DescClause        `| @@`
	Tags        *TagsClause        `| @@`
	Passthrough *PassthroughClause `| @@`
	Inputs      *InputsClause      `| @@`
	Outputs     *OutputsClause     `| @@`
	WhenEnv     *WhenEnvClause     `| @@`
	WhenCmd     *WhenCmdClause     `| @@`
	Private     *PrivateClause     `| @@`
	Preset      *PresetBlock       `| @@`
	Pool        *PoolClause        `| @@`
	Mutex       *MutexClause       `| @@`
	Image       *ImageClause       `| @@`
	Unsafe      *UnsafeClause      `| @@`
	Net         *NetClause         `| @@`
	Vol         *VolClause         `| @@`
	Workflow    *WorkflowClause    `| @@` // workflow { name* } — order for bake up; not first-class
	Daemon      *DaemonBlock       `| @@` // daemon name { ... } — long-running unit; not first-class
}

// WorkflowClause is "workflow" "{" (Ident | ScheduleClause)* "}" — ordered list of target/daemon names and optional schedule.
type WorkflowClause struct {
	Pos     lexer.Position
	Entries []*WorkflowEntry `"workflow" "{" @@* "}"`
}

// WorkflowEntry is one ident (target or daemon name) or a schedule clause.
type WorkflowEntry struct {
	Schedule *ScheduleClause `( @@ |`
	Ident    string         `  @Ident )`
}

// ScheduleClause is "schedule cron \"...\"" or "schedule interval \"...\"" inside a workflow.
type ScheduleClause struct {
	Pos     lexer.Position
	Cron    *QuotedString `  "schedule" "cron" @String`
	Interval *QuotedString `| "schedule" "interval" @String`
}

// ImageClause is "image" followed by ident or quoted string (image reference).
type ImageClause struct {
	Ident  *string       `"image" @Ident |`
	String *QuotedString `"image" @String`
}

// UnsafeClause is "unsafe" — target runs on host even if image is set.
type UnsafeClause struct {
	Pos lexer.Position `"unsafe"`
}

// NetClause is "net" ident — named network to attach; first reference creates.
type NetClause struct {
	Name string `"net" @Ident`
}

// VolClause is "vol" ident or "vol" ident string (name, optional host path). v1.5: name only.
type VolClause struct {
	Name     string        `"vol" @Ident`
	HostPath *QuotedString `( @String )?`
}

// PoolClause is "pool" ident — target uses this named pool (semaphore).
type PoolClause struct {
	Name string `"pool" @Ident`
}

// MutexClause is "mutex" ident — target holds this mutex while running.
type MutexClause struct {
	Name string `"mutex" @Ident`
}

// PresetArgvClause is "argv" "[" (ident|string)* "]".
type PresetArgvClause struct {
	Argv []ExecElem `"argv" "[" ( ( @Ident | @String ) ( "," ( @Ident | @String ) )* )? "]"`
}

// PresetBodyEntry is argv, env, desc, or steps inside a preset block.
type PresetBodyEntry struct {
	Argv  *PresetArgvClause `  @@`
	Env   *EnvBlock         `| @@`
	Desc  *DescClause       `| @@`
	Steps *StepsBlock       `| @@`
}

// PresetBody is the content inside preset { }; at least one entry (argv, env, desc, or steps).
type PresetBody struct {
	Entries []*PresetBodyEntry `@@+`
}

// PresetBlock is "preset" ident "{" body "}".
type PresetBlock struct {
	Pos  lexer.Position
	Name string      `"preset" @Ident`
	Body *PresetBody `"{" @@ "}"`
}

// PrivateClause is "private" inside a target; target is hidden from --list.
type PrivateClause struct {
	Pos lexer.Position `"private"`
}

// WhenEnvClause is "when" "env" ident — condition: env var is set (non-empty).
type WhenEnvClause struct {
	Var string `"when" "env" @Ident`
}

// WhenCmdClause is "when" "cmd" "[" argv "]" — condition: command exits 0.
type WhenCmdClause struct {
	Argv []ExecElem `"when" "cmd" "[" ( ( @Ident | @String ) ( "," ( @Ident | @String ) )* )? "]"`
}

// InputsClause is "inputs" "[" path* "]" for incremental build cache (paths/globs).
type InputsClause struct {
	Paths []ExecElem `"inputs" "[" ( ( @Ident | @String ) ( "," ( @Ident | @String ) )* )? "]"`
}

// OutputsClause is "outputs" "[" path* "]" for incremental build cache (paths/globs).
type OutputsClause struct {
	Paths []ExecElem `"outputs" "[" ( ( @Ident | @String ) ( "," ( @Ident | @String ) )* )? "]"`
}

// DepsClause is "deps" ident ("," ident)* so the list does not consume following keywords.
type DepsClause struct {
	Names []string `"deps" @Ident ( "," @Ident )*`
}

// StepsBlock is "steps" "{" step+ "}".
type StepsBlock struct {
	Steps []*Step `"steps" "{" @@* "}"`
}

// Step is "exec" "[" string* "]" or "cmd" token+ or "shell" runtime script.
type Step struct {
	Exec  *ExecStep  `  "exec" @@`
	Cmd   []string   `| "cmd" @Ident*`
	Shell *ShellStep `| "shell" @@`
}

// ShellStep is "shell" ident string (runtime and script for -c).
type ShellStep struct {
	Runner string       `@Ident`
	Script QuotedString `@String`
}

// ExecStep is "[" (string or ident ("," ...)*)? "]".
type ExecStep struct {
	Argv []ExecElem `"[" ( ( @Ident | @String ) ( "," ( @Ident | @String ) )* )? "]"`
}

// ExecElem captures one argv element (ident or quoted string) and tracks whether it was quoted.
type ExecElem struct {
	Value  string
	Quoted bool
}

// String returns the element value.
func (e ExecElem) String() string { return e.Value }

// Capture implements participle.Capture.
func (e *ExecElem) Capture(values []string) error {
	if len(values) == 0 {
		return nil
	}
	s := values[0]
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		inner := s[1 : len(s)-1]
		var b []byte
		for i := 0; i < len(inner); i++ {
			if inner[i] == '\\' && i+1 < len(inner) && inner[i+1] == '"' {
				b = append(b, '"')
				i++
				continue
			}
			b = append(b, inner[i])
		}
		*e = ExecElem{Value: string(b), Quoted: true}
	} else {
		*e = ExecElem{Value: s, Quoted: false}
	}
	return nil
}

// NewExecElem creates a quoted ExecElem from a string value.
func NewExecElem(v string) ExecElem {
	return ExecElem{Value: v, Quoted: true}
}

// QuotedString captures a double-quoted string and unquotes on capture.
type QuotedString struct {
	Value string
}

// Capture implements participle.Capture; unquotes the token value.
func (q *QuotedString) Capture(values []string) error {
	if len(values) == 0 {
		return nil
	}
	s := values[0]
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		// Minimal unquote: strip " and unescape \"
		inner := s[1 : len(s)-1]
		var b []byte
		for i := 0; i < len(inner); i++ {
			if inner[i] == '\\' && i+1 < len(inner) && inner[i+1] == '"' {
				b = append(b, '"')
				i++
				continue
			}
			b = append(b, inner[i])
		}
		q.Value = string(b)
	} else {
		q.Value = s
	}
	return nil
}

// EnvBlock is "env" "{" (ident string)+ "}".
type EnvBlock struct {
	Pairs []*EnvPair `"env" "{" @@* "}"`
}

// EnvPair is ident then string or ident (key and value).
type EnvPair struct {
	Key   string   `@Ident`
	Value EnvValue `( @Ident | @String )`
}

// EnvValue captures either an ident or a quoted string (for env values).
type EnvValue string

// Capture implements participle.Capture.
func (e *EnvValue) Capture(values []string) error {
	if len(values) == 0 {
		return nil
	}
	s := values[0]
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		inner := s[1 : len(s)-1]
		var b []byte
		for i := 0; i < len(inner); i++ {
			if inner[i] == '\\' && i+1 < len(inner) && inner[i+1] == '"' {
				b = append(b, '"')
				i++
				continue
			}
			b = append(b, inner[i])
		}
		*e = EnvValue(string(b))
	} else {
		*e = EnvValue(s)
	}
	return nil
}

// ArgsClause is "args" ident type (short)? (default)?; simplified for v1.
type ArgsClause struct {
	Name    string `"args" @Ident`
	Type    string `@Ident`
	Short   string `@Ident?`
	Default string `@Ident?`
}

// CwdClause is "cwd" path.
type CwdClause struct {
	Path string `"cwd" @Ident`
}

// DescClause is "desc" string.
type DescClause struct {
	Text QuotedString `"desc" @String`
}

// TagsClause is "tags" ident+.
type TagsClause struct {
	Tags []string `"tags" @Ident*`
}

// PassthroughClause is "passthrough" "step" "=" N (optional "name" string).
type PassthroughClause struct {
	Step int           `"passthrough" "step" "=" @Int`
	Name *QuotedString `( "name" @String )?`
}
