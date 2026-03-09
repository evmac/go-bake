package lint

import (
	"os"
	"testing"

	"github.com/evmac/go-bake/internal/dsl"
)

func mustParse(t *testing.T, src string) *dsl.Bakefile {
	t.Helper()
	ast, err := dsl.Parser.ParseString("", src)
	if err != nil {
		t.Fatal(err)
	}
	return ast
}

func TestPreferExecRule(t *testing.T) {
	cfg := DefaultConfig()
	// Single-line cmd
	ast := mustParse(t, `target build cmd go build ./...`)
	findings, err := Run(ast, "Bakefile", cfg)
	if err != nil {
		t.Fatal(err)
	}
	var prefer int
	for _, f := range findings {
		if f.RuleID == "prefer-exec" {
			prefer++
		}
	}
	if prefer < 1 {
		t.Errorf("expected at least one prefer-exec finding, got %d", prefer)
	}
	// steps { cmd ... }
	ast2 := mustParse(t, `target t { steps { cmd go fmt ./... } }`)
	f2, err := Run(ast2, "Bakefile", cfg)
	if err != nil {
		t.Fatal(err)
	}
	prefer = 0
	for _, f := range f2 {
		if f.RuleID == "prefer-exec" {
			prefer++
			if !f.Fixable {
				t.Error("prefer-exec finding should be fixable")
			}
		}
	}
	if prefer < 1 {
		t.Errorf("expected prefer-exec for cmd step, got %d", prefer)
	}
}

func TestRequireDescRule(t *testing.T) {
	cfg := DefaultConfig()
	ast := mustParse(t, `target build { steps { exec ["true"] } }
target test { desc "run tests" steps { exec ["true"] } }`)
	findings, err := Run(ast, "Bakefile", cfg)
	if err != nil {
		t.Fatal(err)
	}
	var noDesc int
	for _, f := range findings {
		if f.RuleID == "require-desc" && f.Message != "" {
			noDesc++
		}
	}
	if noDesc != 1 {
		t.Errorf("expected one require-desc (build has no desc), got %d", noDesc)
	}
}

func TestBracketsOnlyRule(t *testing.T) {
	cfg := DefaultConfig()
	ast := mustParse(t, `target build cmd go build .`)
	findings, err := Run(ast, "Bakefile", cfg)
	if err != nil {
		t.Fatal(err)
	}
	var brackets int
	for _, f := range findings {
		if f.RuleID == "brackets-only" {
			brackets++
		}
	}
	if brackets < 1 {
		t.Errorf("expected brackets-only for single-line target, got %d", brackets)
	}
}

func TestPreferQuotedExecRule(t *testing.T) {
	cfg := DefaultConfig()
	ast := mustParse(t, `target build { steps { exec [go, build, "./..."] } }`)
	findings, err := Run(ast, "Bakefile", cfg)
	if err != nil {
		t.Fatal(err)
	}
	var quoted int
	for _, f := range findings {
		if f.RuleID == "prefer-quoted-exec" {
			quoted++
			if !f.Fixable {
				t.Error("prefer-quoted-exec should be fixable")
			}
		}
	}
	if quoted < 1 {
		t.Errorf("expected prefer-quoted-exec finding for unquoted exec elements, got %d", quoted)
	}

	ast2 := mustParse(t, `target build { steps { exec ["go", "build", "./..."] } }`)
	findings2, err := Run(ast2, "Bakefile", cfg)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range findings2 {
		if f.RuleID == "prefer-quoted-exec" {
			t.Error("should not flag fully quoted exec elements")
		}
	}
}

func TestPreferQuotedExecRuleInputs(t *testing.T) {
	cfg := DefaultConfig()
	ast := mustParse(t, `target t { inputs [src.go, go.mod] steps { exec ["true"] } }`)
	findings, err := Run(ast, "Bakefile", cfg)
	if err != nil {
		t.Fatal(err)
	}
	var quoted int
	for _, f := range findings {
		if f.RuleID == "prefer-quoted-exec" {
			quoted++
		}
	}
	if quoted < 1 {
		t.Errorf("expected prefer-quoted-exec for unquoted inputs, got %d", quoted)
	}
}

func TestPreferQuotedExecRuleOutputs(t *testing.T) {
	cfg := DefaultConfig()
	ast := mustParse(t, `target t { outputs [out.bin] steps { exec ["true"] } }`)
	findings, err := Run(ast, "Bakefile", cfg)
	if err != nil {
		t.Fatal(err)
	}
	var quoted int
	for _, f := range findings {
		if f.RuleID == "prefer-quoted-exec" {
			quoted++
		}
	}
	if quoted < 1 {
		t.Errorf("expected prefer-quoted-exec for unquoted outputs, got %d", quoted)
	}
}

func TestPreferQuotedExecRulePresetArgv(t *testing.T) {
	cfg := DefaultConfig()
	ast := mustParse(t, `target t { steps { exec ["true"] } preset p { argv [cover] } }`)
	findings, err := Run(ast, "Bakefile", cfg)
	if err != nil {
		t.Fatal(err)
	}
	var quoted int
	for _, f := range findings {
		if f.RuleID == "prefer-quoted-exec" {
			quoted++
		}
	}
	if quoted < 1 {
		t.Errorf("expected prefer-quoted-exec for unquoted preset argv, got %d", quoted)
	}
}

func TestPreferQuotedExecRulePresetSteps(t *testing.T) {
	cfg := DefaultConfig()
	ast := mustParse(t, `target t { steps { exec ["true"] } preset p { steps { exec [go, test] } } }`)
	findings, err := Run(ast, "Bakefile", cfg)
	if err != nil {
		t.Fatal(err)
	}
	var quoted int
	for _, f := range findings {
		if f.RuleID == "prefer-quoted-exec" {
			quoted++
		}
	}
	if quoted < 1 {
		t.Errorf("expected prefer-quoted-exec for unquoted preset steps, got %d", quoted)
	}
}

func TestPreferQuotedExecRuleSingleStep(t *testing.T) {
	cfg := DefaultConfig()
	ast := mustParse(t, `target t { exec [go, build] }`)
	findings, err := Run(ast, "Bakefile", cfg)
	if err != nil {
		t.Fatal(err)
	}
	var quoted int
	for _, f := range findings {
		if f.RuleID == "prefer-quoted-exec" {
			quoted++
		}
	}
	if quoted < 1 {
		t.Errorf("expected prefer-quoted-exec for unquoted single step exec, got %d", quoted)
	}
}

func TestApplyFixesCmdInStepsBlock(t *testing.T) {
	ast := mustParse(t, `target t { steps { cmd go fmt ./... } }`)
	findings, err := Run(ast, "Bakefile", DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	ApplyFixes(ast, findings)
	for _, e := range ast.Entries {
		if e.Target == nil {
			continue
		}
		for _, be := range e.Target.Body.Entries {
			if be.Steps != nil {
				for _, s := range be.Steps.Steps {
					if len(s.Cmd) > 0 {
						t.Error("cmd should have been converted to exec after fix")
					}
					if s.Exec == nil {
						t.Error("exec should be set after fix")
					}
				}
			}
		}
	}
}

func TestRunFromPathNotFound(t *testing.T) {
	cfg := DefaultConfig()
	_, _, err := RunFromPath("/nonexistent/Bakefile", cfg)
	if err == nil {
		t.Fatal("expected error for nonexistent path")
	}
}

func TestRunFromPathValid(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/Bakefile"
	os.WriteFile(path, []byte(`target build { desc "b" steps { exec ["true"] } }`+"\n"), 0644)
	cfg := DefaultConfig()
	findings, ast, err := RunFromPath(path, cfg)
	if err != nil {
		t.Fatalf("RunFromPath: %v", err)
	}
	if ast == nil {
		t.Fatal("expected non-nil AST")
	}
	for _, f := range findings {
		if f.RuleID == "require-desc" {
			t.Errorf("should not flag target with desc: %+v", f)
		}
	}
}

func TestPreferQuotedExecRuleDisabled(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DisableRule("prefer-quoted-exec")
	ast := mustParse(t, `target build { steps { exec [go, build] } }`)
	findings, err := Run(ast, "Bakefile", cfg)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range findings {
		if f.RuleID == "prefer-quoted-exec" {
			t.Error("prefer-quoted-exec should be disabled")
		}
	}
}
