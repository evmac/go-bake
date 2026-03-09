package lint

import (
	"github.com/evmac/go-bake/internal/dsl"
)

// PreferExecRule flags use of "cmd" and suggests "exec" (fixable).
type PreferExecRule struct{}

func (r *PreferExecRule) ID() string { return "prefer-exec" }

func (r *PreferExecRule) Run(filePath string, ast *dsl.Bakefile) ([]Finding, error) {
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
		// Single-line form: target name cmd tok...
		if len(t.CmdTok) > 0 {
			out = append(out, Finding{
				File:       filePath,
				Line:       line,
				Message:    "prefer exec over cmd (single-line target); use target " + t.Name + " { steps { exec [\"...\"] } }",
				Fixable:    true,
				TargetName: t.Name,
				StepIndex:  0,
			})
			continue
		}
		if t.Body == nil {
			continue
		}
		stepIdx := 0
		for _, be := range t.Body.Entries {
			if be.Steps != nil {
				for _, s := range be.Steps.Steps {
					if len(s.Cmd) > 0 {
						out = append(out, Finding{
							File:       filePath,
							Line:       line,
							Message:    "prefer exec over cmd for step " + t.Name,
							Fixable:    true,
							TargetName: t.Name,
							StepIndex:  stepIdx,
						})
					}
					stepIdx++
				}
			}
			if be.Step != nil {
				if len(be.Step.Cmd) > 0 {
					out = append(out, Finding{
						File:       filePath,
						Line:       line,
						Message:    "prefer exec over cmd for step " + t.Name,
						Fixable:    true,
						TargetName: t.Name,
						StepIndex:  stepIdx,
					})
				}
				stepIdx++
			}
		}
	}
	return out, nil
}
