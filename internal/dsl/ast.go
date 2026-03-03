package dsl

// AST types produced by the participle parser. Compiled to config.File by Compile().

// Bakefile is the root AST node.
type Bakefile struct {
	Dotenv   *DotenvLine   `@@?`
	Entries  []*FileEntry  `@@*`
}

// FileEntry is either a Suite or a Target (bracketed or single-line).
type FileEntry struct {
	Suite   *SuiteBlock   `  @@`
	Target  *TargetBlock  `| @@`
}

// DotenvLine is "dotenv" followed by file names.
type DotenvLine struct {
	Files []string `"dotenv" @Ident*`
}

// SuiteBlock is "suite" ident "{" ident+ "}".
type SuiteBlock struct {
	Name    string   `"suite" @Ident`
	Targets []string `"{" @Ident* "}"`
}

// TargetBlock is "target" ident "{" ... "}" (bracketed) or "target" ident "cmd" ... (single-line).
type TargetBlock struct {
	Name   string       `"target" @Ident`
	Body   *TargetBody  `( "{" @@ "}"`
	CmdTok []string     `  | "cmd" @Ident* )` // single-line: cmd token+
}

// TargetBody is the content inside target { }; each clause can appear in any order, args may repeat.
type TargetBody struct {
	Entries []*BodyEntry `@@*`
}

// BodyEntry is one of deps, steps, env, args, cwd, desc, tags, passthrough.
type BodyEntry struct {
	Deps        *DepsClause        `  @@`
	Steps       *StepsBlock        `| @@`
	Env         *EnvBlock          `| @@`
	Args        *ArgsClause        `| @@`
	Cwd         *CwdClause         `| @@`
	Desc        *DescClause        `| @@`
	Tags        *TagsClause        `| @@`
	Passthrough *PassthroughClause `| @@`
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

// ExecElem captures one argv element (ident or quoted string).
type ExecElem string

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
		*e = ExecElem(string(b))
	} else {
		*e = ExecElem(s)
	}
	return nil
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

// PassthroughClause is "passthrough" "step" "=" N.
type PassthroughClause struct {
	Step int `"passthrough" "step" "=" @Int`
}
