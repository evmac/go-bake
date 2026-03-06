package dsl

import (
	"bytes"
	"strings"
	"testing"
)

func TestFormatRoundTrip(t *testing.T) {
	src := `target build {
  desc "build the binary"
  deps generate
  inputs [ "*.go", "go.mod" ]
  outputs [ "bin/app" ]
  steps { exec ["go", "build", "-o", "bin/app", "./..."] }
}
suite dev { build }`
	ast, err := Parser.ParseString("", src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var buf bytes.Buffer
	if err := Format(ast, &buf); err != nil {
		t.Fatalf("format: %v", err)
	}
	formatted := buf.String()
	ast2, err := Parser.ParseString("", formatted)
	if err != nil {
		t.Fatalf("parse formatted: %v\noutput:\n%s", err, formatted)
	}
	cfg1, err := Compile(ast)
	if err != nil {
		t.Fatalf("compile original: %v", err)
	}
	cfg2, err := Compile(ast2)
	if err != nil {
		t.Fatalf("compile formatted: %v", err)
	}
	if len(cfg1.Targets) != len(cfg2.Targets) {
		t.Errorf("targets: got %d, want %d", len(cfg2.Targets), len(cfg1.Targets))
	}
	if len(cfg1.Suites) != len(cfg2.Suites) {
		t.Errorf("suites: got %d, want %d", len(cfg2.Suites), len(cfg1.Suites))
	}
	// Format again should be idempotent.
	var buf2 bytes.Buffer
	if err := Format(ast2, &buf2); err != nil {
		t.Fatalf("format again: %v", err)
	}
	if buf2.String() != formatted {
		t.Errorf("format not idempotent:\nfirst:\n%s\nsecond:\n%s", formatted, buf2.String())
	}
}

func TestFormatNeedsQuoting(t *testing.T) {
	tests := []struct {
		s    string
		want bool
	}{
		{"", true},
		{"a b", true},
		{"--list", true},
		{"*.go", true},
		{"go.mod", false},
		{"./cmd/bake", false},
	}
	for _, tt := range tests {
		if got := needsQuoting(tt.s); got != tt.want {
			t.Errorf("needsQuoting(%q) = %v, want %v", tt.s, got, tt.want)
		}
	}
}

func TestFormatSingleStep(t *testing.T) {
	src := `target fmt { exec ["go", "fmt", "./..."] }`
	ast, err := Parser.ParseString("", src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var buf bytes.Buffer
	if err := Format(ast, &buf); err != nil {
		t.Fatalf("format: %v", err)
	}
	out := strings.TrimSpace(buf.String())
	if !strings.Contains(out, "exec [") || !strings.Contains(out, "go") {
		t.Errorf("expected exec form, got %q", out)
	}
}

// TestFormatProfileRoundTrip exercises writeProfile and writeEnvValue (profile env; quoted value hits writeEnvValue).
func TestFormatProfileRoundTrip(t *testing.T) {
	src := `profile prod {
  env { ENV prod FOO "bar baz" }
}
target build { steps { exec ["true"] } }`
	ast, err := Parser.ParseString("", src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var buf bytes.Buffer
	if err := Format(ast, &buf); err != nil {
		t.Fatalf("format: %v", err)
	}
	formatted := buf.String()
	if !strings.Contains(formatted, "profile prod") || !strings.Contains(formatted, "env {") {
		t.Errorf("format should include profile block: %s", formatted)
	}
	ast2, err := Parser.ParseString("", formatted)
	if err != nil {
		t.Fatalf("parse formatted: %v\n%s", err, formatted)
	}
	cfg, err := Compile(ast2)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	p := cfg.ProfileByName("prod")
	if p == nil || p.Env["ENV"] != "prod" || p.Env["FOO"] != "bar baz" {
		t.Errorf("round-trip profile: got %+v", p)
	}
}

// TestFormatTargetWithBodyEntries exercises writeTarget/writeBodyEntry for deps, desc, steps.
func TestFormatTargetWithBodyEntries(t *testing.T) {
	src := `target all {
  desc "build and test"
  deps build, test
  steps { exec ["sh", "-c", "echo done"] }
}`
	ast, err := Parser.ParseString("", src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var buf bytes.Buffer
	if err := Format(ast, &buf); err != nil {
		t.Fatalf("format: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "desc ") || !strings.Contains(out, "deps ") || !strings.Contains(out, "steps {") {
		t.Errorf("format should include body entries: %s", out)
	}
	ast2, err := Parser.ParseString("", out)
	if err != nil {
		t.Fatalf("parse formatted: %v", err)
	}
	cfg, err := Compile(ast2)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	tgt := cfg.TargetByName("all")
	if tgt == nil || tgt.Desc != "build and test" || len(tgt.Deps) != 2 || len(tgt.Steps) != 1 {
		t.Errorf("round-trip target: got %+v", tgt)
	}
}

// TestFormatTargetWithEnv exercises writeBodyEntry for env block and writeEnvValue.
func TestFormatTargetWithEnv(t *testing.T) {
	src := `target t {
  env { X one FOO "quoted value" }
  steps { exec ["true"] }
}`
	ast, err := Parser.ParseString("", src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var buf bytes.Buffer
	if err := Format(ast, &buf); err != nil {
		t.Fatalf("format: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "env {") || !strings.Contains(out, "X") || !strings.Contains(out, "FOO") {
		t.Errorf("format should include env block: %s", out)
	}
	ast2, err := Parser.ParseString("", out)
	if err != nil {
		t.Fatalf("parse formatted: %v", err)
	}
	cfg, err := Compile(ast2)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	tgt := cfg.TargetByName("t")
	if tgt == nil || tgt.Env["X"] != "one" || tgt.Env["FOO"] != "quoted value" {
		t.Errorf("round-trip target env: got %+v", tgt)
	}
}

func TestFormatTargetWithWhenEnv(t *testing.T) {
	src := `target t { when env FOO steps { exec ["true"] } }`
	ast, err := Parser.ParseString("", src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var buf bytes.Buffer
	if err := Format(ast, &buf); err != nil {
		t.Fatalf("format: %v", err)
	}
	if !strings.Contains(buf.String(), "when env FOO") {
		t.Errorf("format should include when env: %s", buf.String())
	}
}

func TestFormatTargetWithWhenCmd(t *testing.T) {
	src := `target t { when cmd ["test", "-f", "x"] steps { exec ["true"] } }`
	ast, err := Parser.ParseString("", src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var buf bytes.Buffer
	if err := Format(ast, &buf); err != nil {
		t.Fatalf("format: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "when cmd") || !strings.Contains(out, "test") {
		t.Errorf("format should include when cmd: %s", out)
	}
}

func TestFormatTargetWithInputsOutputs(t *testing.T) {
	src := `target t { inputs [ "*.go" ] outputs [ "out" ] steps { exec ["true"] } }`
	ast, err := Parser.ParseString("", src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var buf bytes.Buffer
	if err := Format(ast, &buf); err != nil {
		t.Fatalf("format: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "inputs [") || !strings.Contains(out, "outputs [") {
		t.Errorf("format should include inputs/outputs: %s", out)
	}
}

func TestFormatTargetWithArgsShortAndDefault(t *testing.T) {
	src := `target t { args region string r eu steps { exec ["true"] } }`
	ast, err := Parser.ParseString("", src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var buf bytes.Buffer
	if err := Format(ast, &buf); err != nil {
		t.Fatalf("format: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "args ") || !strings.Contains(out, "region") || !strings.Contains(out, "r ") || !strings.Contains(out, "eu") {
		t.Errorf("format should include args with short and default: %s", out)
	}
}

func TestFormatTargetWithCwd(t *testing.T) {
	src := `target t { cwd subdir steps { exec ["true"] } }`
	ast, err := Parser.ParseString("", src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var buf bytes.Buffer
	if err := Format(ast, &buf); err != nil {
		t.Fatalf("format: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "cwd ") {
		t.Errorf("format should include cwd: %s", out)
	}
}


func TestFormatTargetWithMultipleSteps(t *testing.T) {
	src := `target t {
  steps {
    exec ["echo", "one"]
    exec ["echo", "two"]
  }
}`
	ast, err := Parser.ParseString("", src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var buf bytes.Buffer
	if err := Format(ast, &buf); err != nil {
		t.Fatalf("format: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "one") || !strings.Contains(out, "two") {
		t.Errorf("format should include both steps: %s", out)
	}
}

func TestFormatTargetCmdShorthand(t *testing.T) {
	src := `target build cmd go build ./...`
	ast, err := Parser.ParseString("", src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var buf bytes.Buffer
	if err := Format(ast, &buf); err != nil {
		t.Fatalf("format: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "cmd") || !strings.Contains(out, "go") {
		t.Errorf("format should include cmd shorthand: %s", out)
	}
	cfg, err := Compile(ast)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	tgt := cfg.TargetByName("build")
	if tgt == nil || len(tgt.Steps) != 1 || len(tgt.Steps[0].Argv) != 3 {
		t.Errorf("cmd shorthand should produce one step with argv: %+v", tgt)
	}
}

func TestFormatStepShell(t *testing.T) {
	src := `target t { steps { shell bash "echo hello" } }`
	ast, err := Parser.ParseString("", src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var buf bytes.Buffer
	if err := Format(ast, &buf); err != nil {
		t.Fatalf("format: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "shell") || !strings.Contains(out, "bash") {
		t.Errorf("format should include shell step: %s", out)
	}
}

func TestFormatStepCmd(t *testing.T) {
	src := `target t { steps { cmd go fmt ./... } }`
	ast, err := Parser.ParseString("", src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var buf bytes.Buffer
	if err := Format(ast, &buf); err != nil {
		t.Fatalf("format: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "cmd") {
		t.Errorf("format should include cmd step: %s", out)
	}
}

func TestFormatTargetWithEmptyStepsBlock(t *testing.T) {
	// Build AST with steps block but zero steps to hit writeBodyEntry steps { } path
	ast := &Bakefile{Entries: []*FileEntry{{
		Target: &TargetBlock{
			Name: "t",
			Body: &TargetBody{Entries: []*BodyEntry{
				{Steps: &StepsBlock{Steps: nil}},
			}},
		},
	}}}
	var buf bytes.Buffer
	if err := Format(ast, &buf); err != nil {
		t.Fatalf("format: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "steps {") || !strings.Contains(out, "}") {
		t.Errorf("format should include empty steps block: %s", out)
	}
}

func TestFormatWriteQuotedEscapes(t *testing.T) {
	// Values that need quoting and contain " hit writeQuoted escape path
	src := `target t { desc "say \"hello\"" steps { exec ["true"] } }`
	ast, err := Parser.ParseString("", src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var buf bytes.Buffer
	if err := Format(ast, &buf); err != nil {
		t.Fatalf("format: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, `\"`) {
		t.Errorf("format should escape quotes in desc: %s", out)
	}
}

func TestFormatProfileWithDotenvInBody(t *testing.T) {
	// Build profile AST with dotenv in body to hit writeProfile dotenv branch (parser may not accept profile dotenv + env together)
	p := &ProfileBlock{
		Name: "eu",
		Body: &ProfileBody{
			Entries: []*ProfileBodyEntry{
				{Dotenv: &ProfileDotenvClause{Paths: []ExecElem{ExecElem(".env.eu")}}},
				{Env: &EnvBlock{Pairs: []*EnvPair{{Key: "REGION", Value: EnvValue("eu")}}}},
			},
		},
	}
	ast := &Bakefile{
		Entries: []*FileEntry{
			{Profile: p},
			{Target: mustParseTarget(t, `target x { steps { exec ["true"] } }`)},
		},
	}
	var buf bytes.Buffer
	if err := Format(ast, &buf); err != nil {
		t.Fatalf("format: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "profile eu") || !strings.Contains(out, "dotenv") || !strings.Contains(out, ".env.eu") || !strings.Contains(out, "env {") {
		t.Errorf("format should include profile with dotenv and env: %s", out)
	}
}

func TestFormatWriteBakefileWithImportAndDotenv(t *testing.T) {
	// Build minimal AST with Dotenv and Import to hit writeBakefile branches (same package)
	ast := &Bakefile{
		Dotenv: &DotenvLine{Files: []string{"envfile"}},
		Entries: []*FileEntry{
			{Import: &ImportLine{Path: QuotedString{Value: "other.bake"}}},
			{Target: mustParseTarget(t, `target x { steps { exec ["true"] } }`)},
		},
	}
	var buf bytes.Buffer
	if err := Format(ast, &buf); err != nil {
		t.Fatalf("format: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "dotenv") || !strings.Contains(out, "envfile") {
		t.Errorf("format should include dotenv: %s", out)
	}
	if !strings.Contains(out, "import") || !strings.Contains(out, "other.bake") {
		t.Errorf("format should include import: %s", out)
	}
}

func mustParseTarget(t *testing.T, src string) *TargetBlock {
	t.Helper()
	ast, err := Parser.ParseString("", src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(ast.Entries) != 1 || ast.Entries[0].Target == nil {
		t.Fatal("expected one target entry")
	}
	return ast.Entries[0].Target
}

func TestFormatFormatterStopsOnError(t *testing.T) {
	// Writer that fails after first write - formatter should stop and return err
	ast := &Bakefile{Entries: []*FileEntry{{Target: mustParseTarget(t, `target x { steps { exec ["true"] } }`)}}}
	w := &failingWriter{after: 0}
	err := Format(ast, w)
	if err == nil {
		t.Error("expected error when writer fails")
	}
}

type failingWriter struct{ after int }

func (f *failingWriter) Write(p []byte) (n int, err error) {
	if f.after <= 0 {
		return 0, bytes.ErrTooLarge
	}
	f.after--
	return len(p), nil
}

func (f *failingWriter) WriteString(s string) (n int, err error) {
	if f.after <= 0 {
		return 0, bytes.ErrTooLarge
	}
	f.after--
	return len(s), nil
}
