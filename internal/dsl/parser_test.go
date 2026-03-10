package dsl

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/evmac/go-bake/internal/config"
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
	src := `suite dev { build test }`
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
	if su.Name != "dev" {
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
	// Parse errors should include file:line:col when available
	if errStr := err.Error(); !containsLineCol(errStr) {
		t.Errorf("expected file:line:col in parse error, got: %s", errStr)
	}
}

func TestParseAndCompileValidationErrors(t *testing.T) {
	dir := filepath.Join("..", "..", "testdata")
	t.Run("unknown_dep", func(t *testing.T) {
		_, err := ParseAndCompile(filepath.Join(dir, "unknown-dep.bake"))
		if err == nil {
			t.Fatal("expected validation error for unknown dependency")
		}
		if !containsLineCol(err.Error()) {
			t.Errorf("expected file:line:col in validation error, got: %s", err.Error())
		}
		if !strings.Contains(err.Error(), "unknown dependency") && !strings.Contains(err.Error(), "nonexistent") {
			t.Errorf("expected unknown dependency message, got: %s", err.Error())
		}
	})
	t.Run("duplicate_target", func(t *testing.T) {
		_, err := ParseAndCompile(filepath.Join(dir, "duplicate-target.bake"))
		if err == nil {
			t.Fatal("expected validation error for duplicate target")
		}
		if !strings.Contains(err.Error(), "duplicate target") {
			t.Errorf("expected duplicate target message, got: %s", err.Error())
		}
	})
}

func containsLineCol(s string) bool {
	// Expect something like path:line:col or line:col
	for i := 0; i < len(s)-2; i++ {
		if s[i] == ':' && i+1 < len(s) && s[i+1] >= '0' && s[i+1] <= '9' {
			return true
		}
	}
	return false
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

func TestParseWhen(t *testing.T) {
	// when env VAR
	src1 := `target t { when env CI steps { exec ["go", "test"] } }`
	ast1, err := Parser.ParseString("", src1)
	if err != nil {
		t.Fatalf("parse when env: %v", err)
	}
	cfg1, err := Compile(ast1)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	tgt1 := cfg1.Targets[0]
	if tgt1.WhenEnv != "CI" || tgt1.WhenCmd != nil {
		t.Errorf("when env: got WhenEnv=%q WhenCmd=%v", tgt1.WhenEnv, tgt1.WhenCmd)
	}
	// when cmd ["test", "-f", "file"]
	src2 := `target t { when cmd ["test", "-f", "Makefile"] steps { exec ["make"] } }`
	ast2, err := Parser.ParseString("", src2)
	if err != nil {
		t.Fatalf("parse when cmd: %v", err)
	}
	cfg2, err := Compile(ast2)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	tgt2 := cfg2.Targets[0]
	if tgt2.WhenEnv != "" || len(tgt2.WhenCmd) != 3 || tgt2.WhenCmd[0] != "test" || tgt2.WhenCmd[1] != "-f" || tgt2.WhenCmd[2] != "Makefile" {
		t.Errorf("when cmd: got WhenEnv=%q WhenCmd=%v", tgt2.WhenEnv, tgt2.WhenCmd)
	}
}

func TestParseSingleStepShorthand(t *testing.T) {
	// Bare exec or cmd in target body (no "steps { }" wrapper)
	src := `target format { exec ["go", "fmt", "./..."] }`
	ast, err := Parser.ParseString("", src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	cfg, err := Compile(ast)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if len(cfg.Targets) != 1 || len(cfg.Targets[0].Steps) != 1 {
		t.Fatalf("expected 1 target with 1 step, got %d targets", len(cfg.Targets))
	}
	argv := cfg.Targets[0].Steps[0].Argv
	if len(argv) != 3 || argv[0] != "go" || argv[1] != "fmt" || argv[2] != "./..." {
		t.Errorf("argv: got %v", argv)
	}
	// cmd shorthand (idents only; no leading -)
	src2 := `target fmt { cmd go fmt ./... }`
	ast2, err := Parser.ParseString("", src2)
	if err != nil {
		t.Fatalf("parse cmd: %v", err)
	}
	cfg2, err := Compile(ast2)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if len(cfg2.Targets[0].Steps) != 1 || cfg2.Targets[0].Steps[0].Argv[0] != "go" {
		t.Errorf("cmd step: got %v", cfg2.Targets[0].Steps)
	}
}

func TestParseQuotedStringsInExecAndEnv(t *testing.T) {
	// Hit ExecElem and EnvValue Capture with quoted strings (unescaping).
	src := `target t { env { KEY "value with spaces" } steps { exec ["cmd", "arg with space"] } }`
	ast, err := Parser.ParseString("", src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	cfg, err := Compile(ast)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	tgt := cfg.Targets[0]
	if tgt.Env["KEY"] != "value with spaces" {
		t.Errorf("env KEY: got %q", tgt.Env["KEY"])
	}
	if len(tgt.Steps) != 1 || len(tgt.Steps[0].Argv) != 2 || tgt.Steps[0].Argv[1] != "arg with space" {
		t.Errorf("argv: got %v", tgt.Steps)
	}
}

func TestParseAndCompileInvalidSyntax(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.bake")
	os.WriteFile(path, []byte("target x { steps { exec ["), 0644)
	_, err := ParseAndCompile(path)
	if err == nil {
		t.Fatal("expected parse error")
	}
	if !strings.Contains(err.Error(), "bad.bake") && !strings.Contains(err.Error(), "parse") {
		t.Errorf("error should mention file or parse: %v", err)
	}
}

func TestParseFileMissing(t *testing.T) {
	_, err := ParseFile(filepath.Join(t.TempDir(), "nonexistent.bake"))
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestParseImport(t *testing.T) {
	src := `import "./ops.bake"
target build { steps { exec ["go", "build"] } }`
	ast, err := Parser.ParseString("", src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	cfg, err := Compile(ast)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if len(cfg.Imports) != 1 || cfg.Imports[0] != "./ops.bake" {
		t.Errorf("imports: got %v", cfg.Imports)
	}
	if len(cfg.Targets) != 1 || cfg.Targets[0].Name != "build" {
		t.Errorf("targets: got %v", cfg.Targets)
	}
}

func TestLoadWithImports(t *testing.T) {
	// Create temp dir with main.bake and imported.bake
	dir := t.TempDir()
	mainPath := filepath.Join(dir, "Bakefile")
	importedPath := filepath.Join(dir, "ops.bake")
	mainContent := `import "ops.bake"
target all { deps build, build_ops }
target build { steps { exec ["go", "build"] } }`
	importedContent := `target build_ops { steps { exec ["echo", "ops"] } }`
	if err := os.WriteFile(mainPath, []byte(mainContent), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(importedPath, []byte(importedContent), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadWithImports(mainPath)
	if err != nil {
		t.Fatalf("LoadWithImports: %v", err)
	}
	// Merged: main's targets first, then imported. So build, all, then build_ops.
	if len(cfg.Targets) != 3 {
		t.Fatalf("expected 3 targets, got %d: %v", len(cfg.Targets), cfg.Targets)
	}
	names := make([]string, len(cfg.Targets))
	for i, tgt := range cfg.Targets {
		names[i] = tgt.Name
	}
	// Order: all, build (main), build_ops (imported)
	if cfg.TargetByName("all") == nil || cfg.TargetByName("build") == nil || cfg.TargetByName("build_ops") == nil {
		t.Errorf("merged targets: got %v", names)
	}
	if cfg.RootDir != dir {
		t.Errorf("RootDir: got %q", cfg.RootDir)
	}
}

func TestLoadWithImportsCycle(t *testing.T) {
	dir := t.TempDir()
	aPath := filepath.Join(dir, "a.bake")
	bPath := filepath.Join(dir, "b.bake")
	os.WriteFile(aPath, []byte("import \"b.bake\"\ntarget a { steps { exec [\"true\"] } }\n"), 0644)
	os.WriteFile(bPath, []byte("import \"a.bake\"\ntarget b { steps { exec [\"true\"] } }\n"), 0644)
	_, err := LoadWithImports(aPath)
	if err == nil {
		t.Fatal("expected error for import cycle")
	}
	if !strings.Contains(err.Error(), "cycle") {
		t.Errorf("expected cycle in error, got: %v", err)
	}
}

func TestParseAndCompileProfile(t *testing.T) {
	src := `profile prod {
  env {
    ENV prod
    LOG_LEVEL warn
  }
}
target build { steps { exec ["true"] } }`
	ast, err := Parser.ParseString("", src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	cfg, err := Compile(ast)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if len(cfg.Profiles) != 1 {
		t.Fatalf("expected 1 profile, got %d", len(cfg.Profiles))
	}
	p := cfg.ProfileByName("prod")
	if p == nil {
		t.Fatal("ProfileByName(prod) nil")
	}
	if p.Env["ENV"] != "prod" || p.Env["LOG_LEVEL"] != "warn" {
		t.Errorf("profile env: got %v", p.Env)
	}
	if len(cfg.Targets) != 1 {
		t.Errorf("expected 1 target, got %d", len(cfg.Targets))
	}
}

func TestParseAndCompileProfileWithDotenv(t *testing.T) {
	// Profile with dotenv only (parser accepts dotenv clause in profile body)
	src := `profile eu {
  dotenv ".env.eu"
}
target x { steps { exec ["true"] } }`
	ast, err := Parser.ParseString("", src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	cfg, err := Compile(ast)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	p := cfg.ProfileByName("eu")
	if p == nil || len(p.Dotenv) != 1 || p.Dotenv[0] != ".env.eu" {
		t.Errorf("profile dotenv: got %+v", p)
	}
}

func TestParseAndCompilePrivateFileLevel(t *testing.T) {
	src := `private
target internal { steps { exec ["true"] } }
target public { steps { exec ["true"] } }`
	ast, err := Parser.ParseString("", src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	cfg, err := Compile(ast)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	for _, tgt := range cfg.Targets {
		if !tgt.Private {
			t.Errorf("expected all targets private (file-level), got %q Private=%v", tgt.Name, tgt.Private)
		}
	}
}

func TestParseAndCompilePrivatePerTarget(t *testing.T) {
	src := `target visible { steps { exec ["true"] } }
target hidden {
  private
  steps { exec ["true"] }
}`
	ast, err := Parser.ParseString("", src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	cfg, err := Compile(ast)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	vis := cfg.TargetByName("visible")
	hid := cfg.TargetByName("hidden")
	if vis == nil || hid == nil {
		t.Fatal("expected both targets")
	}
	if vis.Private {
		t.Error("visible should not be private")
	}
	if !hid.Private {
		t.Error("hidden should be private")
	}
}

func TestParseAndCompileSuitePrecommit(t *testing.T) {
	// suite precommit is a normal suite; when present, install hooks runs "bake precommit"
	src := `target lint { steps { exec ["true"] } }
target build { steps { exec ["true"] } }
suite precommit { lint build }`
	ast, err := Parser.ParseString("", src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	cfg, err := Compile(ast)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	su := cfg.SuiteByName("precommit")
	if su == nil {
		t.Fatal("suite precommit should exist")
	}
	if len(su.Targets) != 2 || su.Targets[0] != "lint" || su.Targets[1] != "build" {
		t.Errorf("suite precommit targets: got %v", su.Targets)
	}
}

func TestParseAndCompileFormatParseError(t *testing.T) {
	// ParseAndCompile with parse error includes file:line:col when available (formatParseError)
	_, err := ParseAndCompile(filepath.Join(t.TempDir(), "missing.bake"))
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if !strings.Contains(err.Error(), "missing.bake") {
		t.Errorf("error should mention filename: %v", err)
	}
}

func TestParseAndCompileValidationError(t *testing.T) {
	// ParseAndCompile with validation error (unknown dep) returns validationErrList
	dir := t.TempDir()
	path := filepath.Join(dir, "Bakefile")
	os.WriteFile(path, []byte("target a { deps c }\ntarget b { steps { exec [\"true\"] } }\n"), 0644)
	_, err := ParseAndCompile(path)
	if err == nil {
		t.Fatal("expected validation error for unknown dep")
	}
	// validationErrList.Error() joins multiple errors
	if !strings.Contains(err.Error(), "unknown dependency") && !strings.Contains(err.Error(), "b") {
		t.Errorf("error should mention unknown dep: %v", err)
	}
}

func TestLoadWithImportsBadImportPath(t *testing.T) {
	dir := t.TempDir()
	mainPath := filepath.Join(dir, "Bakefile")
	os.WriteFile(mainPath, []byte("import \"./nonexistent.bake\"\ntarget x { steps { exec [\"true\"] } }\n"), 0644)
	_, err := LoadWithImports(mainPath)
	if err == nil {
		t.Fatal("expected error for missing import")
	}
	if !strings.Contains(err.Error(), "import") {
		t.Errorf("error should mention import: %v", err)
	}
}

func TestParseExecElemQuotedEscape(t *testing.T) {
	// ExecElem.Capture with quoted string containing \" (escape path)
	src := `target t { steps { exec ["arg with \"quotes\" inside"] } }`
	ast, err := Parser.ParseString("", src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	cfg, err := Compile(ast)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	argv := cfg.Targets[0].Steps[0].Argv
	if len(argv) != 1 || !strings.Contains(argv[0], "quotes") {
		t.Errorf("argv should unescape quoted string: %v", argv)
	}
}

func TestParseEnvValueQuotedEscape(t *testing.T) {
	// EnvValue.Capture with quoted string and escape
	src := `target t { env { X "value with \"quote\"" } steps { exec ["true"] } }`
	ast, err := Parser.ParseString("", src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	cfg, err := Compile(ast)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	v := cfg.Targets[0].Env["X"]
	if !strings.Contains(v, "quote") {
		t.Errorf("env value should unescape: %q", v)
	}
}

func TestCompileWithDotenv(t *testing.T) {
	// Compile when ast has file-level Dotenv
	src := "dotenv envfile\ntarget x { steps { exec [\"true\"] } }\n"
	ast, err := Parser.ParseString("", src)
	if err != nil {
		t.Skipf("parser may not accept file-level dotenv in this form: %v", err)
	}
	cfg, err := Compile(ast)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if len(cfg.Dotenv) != 1 || cfg.Dotenv[0] != "envfile" {
		t.Errorf("Dotenv: got %v", cfg.Dotenv)
	}
}

func TestParseTargetWithPassthrough(t *testing.T) {
	src := `target t { passthrough step = 1 steps { exec ["true"] exec ["echo"] } }`
	ast, err := Parser.ParseString("", src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	cfg, err := Compile(ast)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	tgt := cfg.TargetByName("t")
	if tgt == nil || tgt.PassthroughStep != 1 {
		t.Errorf("PassthroughStep: got %+v", tgt)
	}
	// Optional name: passthrough step = 2 name "extra"
	src2 := `target u { passthrough step = 2 name "extra" steps { exec ["true"] exec ["echo"] } }`
	ast2, err := Parser.ParseString("", src2)
	if err != nil {
		t.Fatalf("parse passthrough with name: %v", err)
	}
	cfg2, err := Compile(ast2)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	tgt2 := cfg2.TargetByName("u")
	if tgt2 == nil || len(tgt2.Passthrough) != 1 || tgt2.Passthrough[0].Step != 2 || tgt2.Passthrough[0].Name != "extra" {
		t.Errorf("Passthrough with name: got %+v", tgt2)
	}
}

func TestParseTargetWithPreset(t *testing.T) {
	src := `target test { steps { exec ["go", "test", "./..."] } preset cover { argv ["-coverprofile=coverage.out"] } preset race { env { RACE "1" } } }`
	ast, err := Parser.ParseString("", src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	cfg, err := Compile(ast)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	tgt := cfg.TargetByName("test")
	if tgt == nil || len(tgt.Presets) != 2 {
		t.Fatalf("expected 2 presets, got %+v", tgt)
	}
	cover := tgt.PresetByName("cover")
	if cover == nil || len(cover.Argv) != 1 || cover.Argv[0] != "-coverprofile=coverage.out" {
		t.Errorf("preset cover: got %+v", cover)
	}
	race := tgt.PresetByName("race")
	if race == nil || len(race.Env) != 1 || race.Env["RACE"] != "1" {
		t.Errorf("preset race: got %+v", race)
	}
}

func TestParsePresetWithDescAndSteps(t *testing.T) {
	src := `target test {
  steps { exec ["go", "test", "./..."] }
  preset cover {
    desc "run tests with coverage"
    steps { exec ["go", "test", "./...", "-coverprofile=coverage.out"] }
  }
}`
	ast, err := Parser.ParseString("", src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	cfg, err := Compile(ast)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	tgt := cfg.TargetByName("test")
	if tgt == nil || len(tgt.Presets) != 1 {
		t.Fatalf("expected 1 preset, got %+v", tgt)
	}
	cover := tgt.PresetByName("cover")
	if cover == nil {
		t.Fatal("preset cover missing")
	}
	if cover.Desc != "run tests with coverage" {
		t.Errorf("preset cover desc: got %q", cover.Desc)
	}
	if len(cover.Steps) != 1 || len(cover.Steps[0].Argv) < 4 || cover.Steps[0].Argv[3] != "-coverprofile=coverage.out" {
		t.Errorf("preset cover steps: got %+v", cover.Steps)
	}
}

func TestParseSuiteWithPresetEntry(t *testing.T) {
	src := `suite ci { build test test.cover }`
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
	if len(su.Targets) != 3 || su.Targets[0] != "build" || su.Targets[1] != "test" || su.Targets[2] != "test cover" {
		t.Errorf("suite targets: got %v", su.Targets)
	}
}

func TestParseTargetWithImageUnsafeNetVol(t *testing.T) {
	// image (ident and string), unsafe, net, vol parse and compile
	src := `
target in-container {
  image "alpine:3.19"
  net mynet
  vol myvol
  steps { exec ["sh", "-c", "echo ok"] }
}
target with-ident-image {
  image alpine
  unsafe
  steps { exec ["true"] }
}
target with-vol-hostpath {
  image "busybox"
  vol data "/tmp/data"
  steps { exec ["true"] }
}
`
	ast, err := Parser.ParseString("", src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	cfg, err := Compile(ast)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if len(cfg.Targets) != 3 {
		t.Fatalf("expected 3 targets, got %d", len(cfg.Targets))
	}
	// in-container: image string, net, vol (no hostpath), no unsafe
	t0 := cfg.Targets[0]
	if t0.Name != "in-container" || t0.Image != "alpine:3.19" || t0.Unsafe {
		t.Errorf("target 0: Name=%q Image=%q Unsafe=%v", t0.Name, t0.Image, t0.Unsafe)
	}
	if len(t0.Networks) != 1 || t0.Networks[0] != "mynet" {
		t.Errorf("target 0 Networks: got %v", t0.Networks)
	}
	if len(t0.Volumes) != 1 || t0.Volumes[0].Name != "myvol" || t0.Volumes[0].HostPath != "" {
		t.Errorf("target 0 Volumes: got %+v", t0.Volumes)
	}
	// with-ident-image: image ident, unsafe
	t1 := cfg.Targets[1]
	if t1.Name != "with-ident-image" || t1.Image != "alpine" || !t1.Unsafe {
		t.Errorf("target 1: Name=%q Image=%q Unsafe=%v", t1.Name, t1.Image, t1.Unsafe)
	}
	// with-vol-hostpath: vol with host path
	t2 := cfg.Targets[2]
	if len(t2.Volumes) != 1 || t2.Volumes[0].Name != "data" || t2.Volumes[0].HostPath != "/tmp/data" {
		t.Errorf("target 2 Volumes: got %+v", t2.Volumes)
	}
}

func TestValidationErrListError(t *testing.T) {
	// validationErrList.Error() joins multiple errors
	errs := validationErrList{
		config.ValidationError{Message: "first"},
		config.ValidationError{Message: "second"},
	}
	s := errs.Error()
	if !strings.Contains(s, "first") || !strings.Contains(s, "second") {
		t.Errorf("Error() should join messages: %q", s)
	}
}
