package lint

import (
	"github.com/evmac/go-bake/internal/dsl"
)

// PreferQuotedExecRule flags unquoted elements in exec [] and similar bracketed lists. Fixable via format.
type PreferQuotedExecRule struct{}

func (r *PreferQuotedExecRule) ID() string { return "prefer-quoted-exec" }

func (r *PreferQuotedExecRule) Run(filePath string, ast *dsl.Bakefile) ([]Finding, error) {
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
		if t.Body == nil {
			continue
		}
		for _, be := range t.Body.Entries {
			if hasUnquotedExecElems(be) {
				out = append(out, Finding{
					File:       filePath,
					Line:       line,
					Message:    "prefer quoted strings in exec/inputs/outputs for target " + t.Name + "; run bake format -w",
					Fixable:    true,
					TargetName: t.Name,
				})
				break
			}
		}
	}
	return out, nil
}

func hasUnquotedExecElems(be *dsl.BodyEntry) bool {
	if be == nil {
		return false
	}
	if be.Steps != nil {
		for _, s := range be.Steps.Steps {
			if s.Exec != nil {
				for _, elem := range s.Exec.Argv {
					if !elem.Quoted {
						return true
					}
				}
			}
		}
	}
	if be.Step != nil && be.Step.Exec != nil {
		for _, elem := range be.Step.Exec.Argv {
			if !elem.Quoted {
				return true
			}
		}
	}
	if be.Inputs != nil {
		for _, p := range be.Inputs.Paths {
			if !p.Quoted {
				return true
			}
		}
	}
	if be.Outputs != nil {
		for _, p := range be.Outputs.Paths {
			if !p.Quoted {
				return true
			}
		}
	}
	if be.Preset != nil && be.Preset.Body != nil {
		for _, ve := range be.Preset.Body.Entries {
			if ve.Argv != nil {
				for _, elem := range ve.Argv.Argv {
					if !elem.Quoted {
						return true
					}
				}
			}
			if ve.Steps != nil {
				for _, s := range ve.Steps.Steps {
					if s.Exec != nil {
						for _, elem := range s.Exec.Argv {
							if !elem.Quoted {
								return true
							}
						}
					}
				}
			}
		}
	}
	return false
}
