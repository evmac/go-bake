package dsl

import (
	"fmt"
	"os"

	"github.com/alecthomas/participle/v2"
	"github.com/evmac/go-bake/internal/config"
)

// Parser for Bakefiles.
var Parser = mustBuildParser()

func mustBuildParser() *participle.Parser[Bakefile] {
	p, err := participle.Build[Bakefile](
		participle.Lexer(BakeLexer),
		participle.Elide("Comment", "Whitespace"),
		participle.UseLookahead(2),
	)
	if err != nil {
		panic(err)
	}
	return p
}

// ParseFile parses the Bakefile at path and returns the AST.
func ParseFile(path string) (*Bakefile, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read Bakefile: %w", err)
	}
	return Parser.ParseString(path, string(b))
}

// Compile converts the AST to config.File.
func Compile(ast *Bakefile) (*config.File, error) {
	out := &config.File{}
	if ast.Dotenv != nil {
		out.Dotenv = ast.Dotenv.Files
	}
	for _, e := range ast.Entries {
		if e.Suite != nil {
			out.Suites = append(out.Suites, &config.Suite{
				Name:    e.Suite.Name,
				Targets: e.Suite.Targets,
			})
			continue
		}
		if e.Target != nil {
			t, err := compileTarget(e.Target)
			if err != nil {
				return nil, err
			}
			out.Targets = append(out.Targets, t)
		}
	}
	return out, nil
}

func compileTarget(t *TargetBlock) (*config.Target, error) {
	tgt := &config.Target{Name: t.Name}
	if t.Body != nil {
		for _, e := range t.Body.Entries {
			if e == nil {
				continue
			}
			if e.Deps != nil {
				tgt.Deps = e.Deps.Names
			}
			if e.Steps != nil {
				for _, s := range e.Steps.Steps {
					step, err := compileStep(s)
					if err != nil {
						return nil, err
					}
					tgt.Steps = append(tgt.Steps, step)
				}
			}
			if e.Env != nil {
				if tgt.Env == nil {
					tgt.Env = make(map[string]string)
				}
				for _, p := range e.Env.Pairs {
					tgt.Env[p.Key] = string(p.Value)
				}
			}
			if e.Args != nil {
				tgt.Args = append(tgt.Args, config.ArgDecl{
					Name:    e.Args.Name,
					Type:    e.Args.Type,
					Short:   e.Args.Short,
					Default: e.Args.Default,
				})
			}
			if e.Cwd != nil {
				tgt.Cwd = e.Cwd.Path
			}
			if e.Desc != nil {
				tgt.Desc = e.Desc.Text.Value
			}
			if e.Tags != nil {
				tgt.Tags = e.Tags.Tags
			}
			if e.Passthrough != nil {
				tgt.PassthroughStep = e.Passthrough.Step
			}
		}
	}
	if len(t.CmdTok) > 0 {
		// Single-line form: cmd token+ → one step with argv
		tgt.Steps = append(tgt.Steps, config.Step{Argv: t.CmdTok})
	}
	return tgt, nil
}

func compileStep(s *Step) (config.Step, error) {
	if s.Exec != nil {
		argv := make([]string, 0, len(s.Exec.Argv))
		for _, elem := range s.Exec.Argv {
			argv = append(argv, string(elem))
		}
		return config.Step{Argv: argv}, nil
	}
	if len(s.Cmd) > 0 {
		return config.Step{Argv: s.Cmd}, nil
	}
	if s.Shell != nil {
		return config.Step{Runner: s.Shell.Runner, Argv: []string{"-c", s.Shell.Script.Value}}, nil
	}
	return config.Step{}, nil
}

// ParseAndCompile parses path and compiles to config.File.
func ParseAndCompile(path string) (*config.File, error) {
	ast, err := ParseFile(path)
	if err != nil {
		return nil, err
	}
	return Compile(ast)
}

