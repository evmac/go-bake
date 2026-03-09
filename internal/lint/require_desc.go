package lint

import (
	"github.com/evmac/go-bake/internal/dsl"
)

// RequireDescRule warns when a target has no desc (not fixable).
type RequireDescRule struct{}

func (r *RequireDescRule) ID() string { return "require-desc" }

func (r *RequireDescRule) Run(filePath string, ast *dsl.Bakefile) ([]Finding, error) {
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
		hasDesc := false
		if t.Body != nil {
			for _, be := range t.Body.Entries {
				if be.Desc != nil {
					hasDesc = true
					break
				}
			}
		}
		if !hasDesc {
			out = append(out, Finding{
				File:    filePath,
				Line:    line,
				Message: "target " + t.Name + " has no desc",
				Fixable: false,
			})
		}
	}
	return out, nil
}
