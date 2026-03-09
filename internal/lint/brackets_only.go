package lint

import (
	"github.com/evmac/go-bake/internal/dsl"
)

// BracketsOnlyRule flags single-line or bare-step targets and suggests bracketed form (fixable via format).
type BracketsOnlyRule struct{}

func (r *BracketsOnlyRule) ID() string { return "brackets-only" }

func (r *BracketsOnlyRule) Run(filePath string, ast *dsl.Bakefile) ([]Finding, error) {
	var out []Finding
	for _, e := range ast.Entries {
		if e.Target == nil {
			continue
		}
		t := e.Target
		line := 0
		if t.Pos.Line > 0 {
			line = t.Pos.Line
		}
		// Single-line: target name cmd ...
		if len(t.CmdTok) > 0 {
			out = append(out, Finding{
				File:    filePath,
				Line:    line,
				Message: "use bracketed form: target " + t.Name + " { steps { ... } }",
				Fixable: true,
			})
			continue
		}
		if t.Body == nil {
			continue
		}
		// Bare step(s) without "steps { }" wrapper
		for _, be := range t.Body.Entries {
			if be.Step != nil {
				out = append(out, Finding{
					File:    filePath,
					Line:    line,
					Message: "use steps { } wrapper for steps in target " + t.Name,
					Fixable: true,
				})
				break
			}
		}
	}
	return out, nil
}
