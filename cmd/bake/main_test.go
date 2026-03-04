package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestBakeListExitCode(t *testing.T) {
	// Build bake binary and run bake --list from repo root; assert we get exit 0 or 2 (no Bakefile yet is ok for bootstrap)
	exe := buildBake(t)
	dir := repoRoot(t)
	cmd := exec.Command(exe, "--list")
	cmd.Dir = dir
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 2 {
			// Expected when no Bakefile or not implemented
			return
		}
		t.Logf("output: %s", out)
		t.Fatalf("bake --list: %v", err)
	}
	t.Logf("bake --list: %s", out)
}

func TestLoadConfig(t *testing.T) {
	dir := t.TempDir()
	bakefile := filepath.Join(dir, "Bakefile")
	content := []byte("target build { steps { exec [\"true\"] } }\n")
	if err := os.WriteFile(bakefile, content, 0644); err != nil {
		t.Fatal(err)
	}
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(orig)
	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if cfg == nil {
		t.Fatal("loadConfig returned nil config")
	}
	if cfg.RootDir == "" {
		t.Error("RootDir empty")
	}
	// dir may be symlinked; ensure config points at a dir that contains our Bakefile
	if _, err := os.Stat(filepath.Join(cfg.RootDir, "Bakefile")); err != nil {
		t.Errorf("Bakefile not under RootDir: %v", err)
	}
	if cfg.TargetByName("build") == nil {
		t.Error("expected target build")
	}
	if cfg.DefaultTargetName() != "build" {
		t.Errorf("DefaultTargetName: got %q", cfg.DefaultTargetName())
	}
}

func TestRunListWithBakefile(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte("target build { steps { exec [\"true\"] } }\nsuite local { build }\nsuite ci { build }\n"), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	if err := runList(false); err != nil {
		t.Errorf("runList: %v", err)
	}
	t.Setenv("CI", "1")
	if err := runList(true); err != nil {
		t.Errorf("runList(ci): %v", err)
	}
}

func TestRunDefault(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte("target default { steps { exec [\"true\"] } }\n"), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	if err := runDefault(); err != nil {
		t.Errorf("runDefault: %v", err)
	}
}

func TestRunTarget(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte("target build { steps { exec [\"true\"] } }\n"), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	cfg, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	tgt := cfg.TargetByName("build")
	if err := runTarget(cfg, "build", tgt, nil, nil, nil); err != nil {
		t.Errorf("runTarget: %v", err)
	}
}

func TestRunTargetWithDeclaredArgs(t *testing.T) {
	dir := t.TempDir()
	// Use same args format as testdata/with-args.bake so "steps" is not consumed as args' default
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte("target build { args x string x v steps { exec [\"true\"] } }\n"), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	cfg, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	tgt := cfg.TargetByName("build")
	declared := map[string]string{"x": "v"}
	if err := runTarget(cfg, "build", tgt, declared, map[string]string{"live": "k"}, nil); err != nil {
		t.Errorf("runTarget with declared+live: %v", err)
	}
}

func TestRunWhy(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte("target build { steps { exec [\"true\"] } }\n"), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	if err := runWhy("build", nil); err != nil {
		t.Errorf("runWhy: %v", err)
	}
	// With extra args to hit ParseArgs(tgt, args[1:]) path
	if err := runWhy("build", []string{"build", "--live", "x"}); err != nil {
		t.Errorf("runWhy with args: %v", err)
	}
}

func TestRunDryRun(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte("target build { steps { exec [\"go\", \"build\"] } }\n"), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	cfg, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	runDryRun(cfg, "build", cfg.TargetByName("build"), nil, nil, nil)
}

func TestRunExplain(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte("target build { desc \"build binary\" steps { exec [\"go\", \"build\"] } }\n"), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	if err := runExplain("build", false); err != nil {
		t.Errorf("runExplain: %v", err)
	}
}

func TestRunTargetNoSteps(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte("target noop { }\n"), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	cfg, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	tgt := cfg.TargetByName("noop")
	if err := runTarget(cfg, "noop", tgt, nil, nil, nil); err != nil {
		t.Errorf("runTarget (no steps): %v", err)
	}
}

func TestRunInstall(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte("target build { steps { exec [\"true\"] } }\n"), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	if err := runInstall(); err != nil {
		t.Errorf("runInstall: %v", err)
	}
	binDir := filepath.Join(dir, ".bake", "bin")
	if fi, err := os.Stat(binDir); err != nil || !fi.IsDir() {
		t.Errorf(".bake/bin not created or not dir: %v", err)
	}
}

func TestRunMainList(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte("target build { steps { exec [\"true\"] } }\nsuite local { build }\n"), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	code, err := RunMain([]string{"--list"})
	if err != nil {
		t.Fatalf("RunMain --list: %v", err)
	}
	if code != 0 {
		t.Errorf("expected exit code 0, got %d", code)
	}
}

func TestRunMainDefaultTarget(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte("target default { steps { exec [\"true\"] } }\n"), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	code, err := RunMain([]string{})
	if err != nil {
		t.Fatalf("RunMain (default): %v", err)
	}
	if code != 0 {
		t.Errorf("expected exit code 0, got %d", code)
	}
}

func TestRunMainWhy(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte("target build { steps { exec [\"true\"] } }\n"), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	code, err := RunMain([]string{"--why", "build"})
	if err != nil {
		t.Fatalf("RunMain --why: %v", err)
	}
	if code != 0 {
		t.Errorf("expected exit code 0, got %d", code)
	}
}

func TestRunMainNoBakefile(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	code, err := RunMain([]string{"build"})
	if err == nil {
		t.Fatal("expected error when no Bakefile")
	}
	if code != 2 {
		t.Errorf("expected exit code 2, got %d", code)
	}
}

func TestRunMainExplain(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte("target build { steps { exec [\"true\"] } }\n"), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	code, err := RunMain([]string{"--explain", "build"})
	if err != nil {
		t.Fatalf("RunMain --explain: %v", err)
	}
	if code != 0 {
		t.Errorf("expected exit code 0, got %d", code)
	}
}

func TestRunMainDryRun(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte("target build { steps { exec [\"go\", \"build\"] } }\n"), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	code, err := RunMain([]string{"--dry-run", "build"})
	if err != nil {
		t.Fatalf("RunMain --dry-run: %v", err)
	}
	if code != 0 {
		t.Errorf("expected exit code 0, got %d", code)
	}
}

func TestRunMainRunTarget(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte("target build { steps { exec [\"true\"] } }\n"), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	code, err := RunMain([]string{"build"})
	if err != nil {
		t.Fatalf("RunMain build: %v", err)
	}
	if code != 0 {
		t.Errorf("expected exit code 0, got %d", code)
	}
}

func TestRunMainRunSuite(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte("target a { steps { exec [\"true\"] } }\ntarget b { deps a steps { exec [\"true\"] } }\nsuite s { a b }\n"), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	code, err := RunMain([]string{"s"})
	if err != nil {
		t.Fatalf("RunMain suite: %v", err)
	}
	if code != 0 {
		t.Errorf("expected exit code 0, got %d", code)
	}
}

func TestRunMainInstall(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte("target build { steps { exec [\"true\"] } }\n"), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	code, err := RunMain([]string{"install"})
	if err != nil {
		t.Fatalf("RunMain install: %v", err)
	}
	if code != 0 {
		t.Errorf("expected exit code 0, got %d", code)
	}
}

func TestRunMainUnknownTarget(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte("target build { steps { exec [\"true\"] } }\n"), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	code, err := RunMain([]string{"nonexistent"})
	if err == nil {
		t.Fatal("expected error for unknown target")
	}
	if code != 2 {
		t.Errorf("expected exit code 2, got %d", code)
	}
}

func TestBakeNoTargetShowsError(t *testing.T) {
	exe := buildBake(t)
	// Run from a dir with no Bakefile so we get "no Bakefile" or "no target" error
	dir := t.TempDir()
	cmd := exec.Command(exe)
	cmd.Dir = dir
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected non-zero exit when no target and no Bakefile, got output: %s", out)
	}
	if exitErr, ok := err.(*exec.ExitError); !ok || exitErr.ExitCode() != 2 {
		t.Logf("output: %s", out)
		t.Fatalf("expected exit 2, got: %v", err)
	}
}

func buildBake(t *testing.T) string {
	t.Helper()
	dir := repoRoot(t)
	exe := filepath.Join(dir, "bake.test.exe")
	cmd := exec.Command("go", "build", "-o", exe, ".")
	cmd.Dir = filepath.Join(dir, "cmd", "bake")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	return exe
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	// From cmd/bake we need to go up to repo root
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find repo root (go.mod)")
		}
		dir = parent
	}
}
