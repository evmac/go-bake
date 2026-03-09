package lint

import (
	"github.com/evmac/go-bake/internal/dsl"
)

// Rule runs over a Bakefile AST and returns findings.
type Rule interface {
	ID() string
	Run(filePath string, ast *dsl.Bakefile) ([]Finding, error)
}

// Registry holds all built-in rules.
var Registry = []Rule{
	&PreferExecRule{},
	&BracketsOnlyRule{},
	&RequireDescRule{},
	&PreferQuotedExecRule{},
}

// Run runs all enabled rules on the AST and returns findings.
func Run(ast *dsl.Bakefile, filePath string, cfg *Config) ([]Finding, error) {
	var out []Finding
	for _, rule := range Registry {
		if !cfg.RuleEnabled(rule.ID()) {
			continue
		}
		findings, err := rule.Run(filePath, ast)
		if err != nil {
			return nil, err
		}
		for _, f := range findings {
			f.RuleID = rule.ID()
			out = append(out, f)
		}
	}
	return out, nil
}
