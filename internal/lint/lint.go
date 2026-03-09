package lint

import (
	"fmt"
	"io"

	"github.com/evmac/go-bake/internal/dsl"
)

// ApplyFixes mutates ast to apply fixable findings (prefer-exec: convert cmd to exec).
// Caller should then run dsl.Format(ast) and write to file for full fix (brackets-only is fixed by format).
func ApplyFixes(ast *dsl.Bakefile, findings []Finding) {
	// Group by target for prefer-exec step fixes
	type key struct {
		target  string
		stepIdx int
	}
	preferExec := make(map[key]bool)
	for _, f := range findings {
		if f.RuleID == "prefer-exec" && f.Fixable && f.TargetName != "" {
			preferExec[key{f.TargetName, f.StepIndex}] = true
		}
	}
	for _, e := range ast.Entries {
		if e.Target == nil {
			continue
		}
		t := e.Target
		// Single-line form: convert to bracketed with one exec step
		if len(t.CmdTok) > 0 {
			argv := make([]dsl.ExecElem, len(t.CmdTok))
			for i, tok := range t.CmdTok {
				argv[i] = dsl.NewExecElem(tok)
			}
			t.CmdTok = nil
			t.Body = &dsl.TargetBody{
				Entries: []*dsl.BodyEntry{{
					Steps: &dsl.StepsBlock{
						Steps: []*dsl.Step{{Exec: &dsl.ExecStep{Argv: argv}}},
					},
				}},
			}
			continue
		}
		if t.Body == nil {
			continue
		}
		stepIdx := 0
		for _, be := range t.Body.Entries {
			if be.Steps != nil {
				for _, s := range be.Steps.Steps {
					if len(s.Cmd) > 0 && preferExec[key{t.Name, stepIdx}] {
						argv := make([]dsl.ExecElem, len(s.Cmd))
						for i, tok := range s.Cmd {
							argv[i] = dsl.NewExecElem(tok)
						}
						s.Cmd = nil
						s.Exec = &dsl.ExecStep{Argv: argv}
					}
					stepIdx++
				}
			}
			if be.Step != nil {
				if len(be.Step.Cmd) > 0 && preferExec[key{t.Name, stepIdx}] {
					argv := make([]dsl.ExecElem, len(be.Step.Cmd))
					for i, tok := range be.Step.Cmd {
						argv[i] = dsl.NewExecElem(tok)
					}
					be.Step.Cmd = nil
					be.Step.Exec = &dsl.ExecStep{Argv: argv}
				}
				stepIdx++
			}
		}
	}
}

// RunFromPath loads the Bakefile at path, runs the linter with cfg, and returns findings.
func RunFromPath(path string, cfg *Config) ([]Finding, *dsl.Bakefile, error) {
	ast, err := dsl.ParseFile(path)
	if err != nil {
		return nil, nil, err
	}
	findings, err := Run(ast, path, cfg)
	if err != nil {
		return nil, nil, err
	}
	return findings, ast, nil
}

// PrintFindings writes findings to w in human-readable form (path:line: message or path:line:col: message).
func PrintFindings(w io.Writer, findings []Finding, jsonMode bool) error {
	if jsonMode {
		return printFindingsJSON(w, findings)
	}
	for _, f := range findings {
		if f.Column > 0 {
			fmt.Fprintf(w, "%s:%d:%d: %s\n", f.File, f.Line, f.Column, f.Message)
		} else {
			fmt.Fprintf(w, "%s:%d: %s\n", f.File, f.Line, f.Message)
		}
	}
	return nil
}

func printFindingsJSON(w io.Writer, findings []Finding) error {
	enc := jsonEncoder{w}
	for _, f := range findings {
		enc.writeFinding(f)
	}
	return nil
}

type jsonEncoder struct{ w io.Writer }

func (e jsonEncoder) writeFinding(f Finding) {
	// NDJSON: one JSON object per line
	fmt.Fprintf(e.w, `{"file":%q,"line":%d,"column":%d,"message":%q,"rule":%q,"fixable":%t}`+"\n",
		f.File, f.Line, f.Column, f.Message, f.RuleID, f.Fixable)
}
