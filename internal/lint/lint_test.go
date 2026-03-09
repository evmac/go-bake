package lint

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/evmac/go-bake/internal/dsl"
)

func TestApplyFixesPreferExec(t *testing.T) {
	src := `target t { steps { cmd go fmt ./... } }`
	ast, err := dsl.Parser.ParseString("", src)
	if err != nil {
		t.Fatal(err)
	}
	findings, err := Run(ast, "Bakefile", DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	ApplyFixes(ast, findings)
	var buf bytes.Buffer
	if err := dsl.Format(ast, &buf); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !contains(out, "exec [") {
		t.Errorf("expected exec form after fix, got %s", out)
	}
	if contains(out, "cmd go") {
		t.Errorf("cmd should be converted to exec, got %s", out)
	}
}

func TestApplyFixesSingleLineTarget(t *testing.T) {
	src := `target build cmd go build .`
	ast, err := dsl.Parser.ParseString("", src)
	if err != nil {
		t.Fatal(err)
	}
	findings, err := Run(ast, "Bakefile", DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	ApplyFixes(ast, findings)
	var buf bytes.Buffer
	if err := dsl.Format(ast, &buf); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !contains(out, "steps") || !contains(out, "exec [") {
		t.Errorf("expected bracketed form with exec, got %s", out)
	}
	if contains(out, "cmd go") {
		t.Errorf("single-line cmd should be converted, got %s", out)
	}
}

func TestRunFromPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "Bakefile")
	if err := os.WriteFile(path, []byte("target build { steps { cmd true } }\n"), 0644); err != nil {
		t.Fatal(err)
	}
	findings, _, err := RunFromPath(path, DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) == 0 {
		t.Error("expected at least one finding for cmd step")
	}
}

func contains(s, sub string) bool {
	return bytes.Contains([]byte(s), []byte(sub))
}

func TestPrintFindingsHuman(t *testing.T) {
	var buf bytes.Buffer
	findings := []Finding{
		{File: "Bakefile", Line: 1, Message: "prefer exec", RuleID: "prefer-exec"},
		{File: "Bakefile", Line: 2, Column: 3, Message: "no desc", RuleID: "require-desc"},
	}
	if err := PrintFindings(&buf, findings, false); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "Bakefile:1:") || !strings.Contains(out, "prefer exec") {
		t.Errorf("expected file:line: message, got %s", out)
	}
	if !strings.Contains(out, "2:3:") {
		t.Errorf("expected column when set, got %s", out)
	}
}

func TestPrintFindingsJSON(t *testing.T) {
	var buf bytes.Buffer
	findings := []Finding{
		{File: "Bakefile", Line: 1, Message: "msg", RuleID: "prefer-exec", Fixable: true},
	}
	if err := PrintFindings(&buf, findings, true); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, `"file":"Bakefile"`) || !strings.Contains(out, `"rule":"prefer-exec"`) || !strings.Contains(out, `"fixable":true`) {
		t.Errorf("expected NDJSON, got %s", out)
	}
}
