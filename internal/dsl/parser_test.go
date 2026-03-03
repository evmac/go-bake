package dsl

import (
	"path/filepath"
	"testing"
)

func TestParseMinimal(t *testing.T) {
	dir := filepath.Join("..", "..", "testdata")
	cfg, err := ParseAndCompile(filepath.Join(dir, "minimal.bake"))
	if err != nil {
		t.Fatalf("parse minimal.bake: %v", err)
	}
	if len(cfg.Targets) != 1 {
		t.Fatalf("expected 1 target, got %d", len(cfg.Targets))
	}
	tgt := cfg.Targets[0]
	if tgt.Name != "build" {
		t.Errorf("target name: got %q", tgt.Name)
	}
	if len(tgt.Steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(tgt.Steps))
	}
	argv := tgt.Steps[0].Argv
	if len(argv) != 3 || argv[0] != "go" || argv[1] != "build" || argv[2] != "./..." {
		t.Errorf("argv: got %v", argv)
	}
}

func TestParseBracketed(t *testing.T) {
	// target build { steps { exec ["go","build","./..."] } }
	src := `target build { steps { exec ["go","build","./..."] } }`
	ast, err := Parser.ParseString("", src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	cfg, err := Compile(ast)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if len(cfg.Targets) != 1 {
		t.Fatalf("expected 1 target, got %d", len(cfg.Targets))
	}
	tgt := cfg.Targets[0]
	if tgt.Name != "build" {
		t.Errorf("target name: got %q", tgt.Name)
	}
	if len(tgt.Steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(tgt.Steps))
	}
	argv := tgt.Steps[0].Argv
	if len(argv) != 3 || argv[0] != "go" || argv[1] != "build" || argv[2] != "./..." {
		t.Errorf("argv: got %v", argv)
	}
}

func TestParseSuite(t *testing.T) {
	src := `suite local { build test }`
	ast, err := Parser.ParseString("", src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	cfg, err := Compile(ast)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if len(cfg.Suites) != 1 {
		t.Fatalf("expected 1 suite, got %d", len(cfg.Suites))
	}
	su := cfg.Suites[0]
	if su.Name != "local" {
		t.Errorf("suite name: got %q", su.Name)
	}
	if len(su.Targets) != 2 || su.Targets[0] != "build" || su.Targets[1] != "test" {
		t.Errorf("suite targets: got %v", su.Targets)
	}
}

func TestParseShellStep(t *testing.T) {
	src := `target foo { steps { shell bash "echo hello | cat" } }`
	ast, err := Parser.ParseString("", src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	cfg, err := Compile(ast)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	tgt := cfg.Targets[0]
	if len(tgt.Steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(tgt.Steps))
	}
	step := tgt.Steps[0]
	if step.Runner != "bash" {
		t.Errorf("runner: got %q", step.Runner)
	}
	if len(step.Argv) != 2 || step.Argv[0] != "-c" || step.Argv[1] != "echo hello | cat" {
		t.Errorf("argv: got %v", step.Argv)
	}
}

func TestParseInvalidExpectError(t *testing.T) {
	dir := filepath.Join("..", "..", "testdata")
	_, err := ParseAndCompile(filepath.Join(dir, "invalid-syntax.bake"))
	if err == nil {
		t.Fatal("expected parse error for invalid-syntax.bake")
	}
}

func TestParseDepsAndEnv(t *testing.T) {
	// First test without env block to isolate steps
	src1 := `target build { deps generate steps { exec ["go","build"] } }`
	ast, err := Parser.ParseString("", src1)
	if err != nil {
		t.Fatalf("parse (no env): %v", err)
	}
	cfg, err := Compile(ast)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	tgt := cfg.Targets[0]
	if len(tgt.Deps) != 1 || tgt.Deps[0] != "generate" {
		t.Errorf("deps: got %v", tgt.Deps)
	}
	if len(tgt.Steps) != 1 || len(tgt.Steps[0].Argv) != 2 {
		t.Errorf("steps: got %v", tgt.Steps)
	}

	// With env block
	src2 := `target build { deps generate env { GOOS linux GOARCH amd64 } steps { exec ["go","build"] } }`
	ast, err = Parser.ParseString("", src2)
	if err != nil {
		t.Fatalf("parse (with env): %v", err)
	}
	cfg, err = Compile(ast)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	tgt = cfg.Targets[0]
	if tgt.Env["GOOS"] != "linux" || tgt.Env["GOARCH"] != "amd64" {
		t.Errorf("env: got %v", tgt.Env)
	}
}
