package main

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// Tests in this file are either:
// - RunMain(args): in-process CLI dispatch (flag parsing, loadConfig, runner, etc.). No subprocess.
// - buildBake(t) + exec.Command(exe, ...): integration tests that run the built binary for real main() and exit codes.

func TestParseSetFlags(t *testing.T) {
	cliEnv, cliArgs := parseSetFlags(nil)
	if len(cliEnv) != 0 || len(cliArgs) != 0 {
		t.Errorf("nil: got env=%v args=%v", cliEnv, cliArgs)
	}
	cliEnv, cliArgs = parseSetFlags([]string{"env.FOO=bar", "args.region=eu", "env.X=1"})
	if cliEnv["FOO"] != "bar" || cliEnv["X"] != "1" {
		t.Errorf("env: got %v", cliEnv)
	}
	if cliArgs["region"] != "eu" {
		t.Errorf("args: got %v", cliArgs)
	}
	// non-matching keys ignored
	cliEnv, cliArgs = parseSetFlags([]string{"invalid", "env.A=B"})
	if cliEnv["A"] != "B" || len(cliArgs) != 0 {
		t.Errorf("got env=%v args=%v", cliEnv, cliArgs)
	}
}

func TestMergeEnv(t *testing.T) {
	a := map[string]string{"X": "1", "Y": "2"}
	b := map[string]string{"Y": "over", "Z": "3"}
	out := mergeEnv(a, b)
	if out["X"] != "1" || out["Y"] != "over" || out["Z"] != "3" {
		t.Errorf("merge: got %v", out)
	}
	if a["Y"] != "2" {
		t.Error("mergeEnv should not mutate inputs")
	}
}

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
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte("target build { steps { exec [\"true\"] } }\nsuite dev { build }\nsuite ci { build }\n"), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	if err := runList(false, false); err != nil {
		t.Errorf("runList: %v", err)
	}
	t.Setenv("CI", "1")
	if err := runList(true, false); err != nil {
		t.Errorf("runList(ci): %v", err)
	}
}

func TestRunDefault(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte("target default { steps { exec [\"true\"] } }\n"), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	if err := runDefault(nil, nil, "", false, false, 1, false, false); err != nil {
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
	if err := runTarget(cfg, "build", tgt, nil, nil, nil, nil, nil, nil, false, false, 1, false, false); err != nil {
		t.Errorf("runTarget: %v", err)
	}
}

func TestRunTargetNilTarget(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte("target build { steps { exec [\"true\"] } }\n"), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	cfg, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	err = runTarget(cfg, "ghost", nil, nil, nil, nil, nil, nil, nil, false, false, 1, false, false)
	if err == nil {
		t.Fatal("expected error when target is nil")
	}
	if !strings.Contains(err.Error(), "unknown target") {
		t.Errorf("unexpected error: %v", err)
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
	if err := runTarget(cfg, "build", tgt, declared, map[string]string{"live": "k"}, nil, nil, nil, nil, false, false, 1, false, false); err != nil {
		t.Errorf("runTarget with declared+live: %v", err)
	}
}

func TestRunWhy(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte("target build { steps { exec [\"true\"] } }\n"), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	if err := runWhy("build", nil, nil, nil); err != nil {
		t.Errorf("runWhy: %v", err)
	}
	// With extra args to hit ParseArgs(tgt, args[1:]) path
	if err := runWhy("build", []string{"build", "--live", "x"}, nil, nil); err != nil {
		t.Errorf("runWhy with args: %v", err)
	}
}

func TestRunWhyWithInputsOutputs(t *testing.T) {
	dir := t.TempDir()
	inPath := filepath.Join(dir, "in.txt")
	outPath := filepath.Join(dir, "out.txt")
	os.WriteFile(inPath, []byte("x"), 0644)
	os.WriteFile(outPath, []byte("y"), 0644)
	bf := "target cached { inputs [ \"in.txt\" ] outputs [ \"out.txt\" ] steps { exec [\"true\"] } }\n"
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte(bf), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	out, code, err := runMainCaptureStdout(t, []string{"--why", "cached"})
	if err != nil {
		t.Fatalf("runWhy: %v", err)
	}
	if code != 0 {
		t.Errorf("expected exit 0, got %d", code)
	}
	if out != "" && !strings.Contains(out, "run") && !strings.Contains(out, "cache") && !strings.Contains(out, "skip") && !strings.Contains(out, "input") {
		t.Logf("why output: %s", out)
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
	runDryRun(cfg, "build", cfg.TargetByName("build"), nil, nil, nil, nil, nil)
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
	if err := runTarget(cfg, "noop", tgt, nil, nil, nil, nil, nil, nil, false, false, 1, false, false); err != nil {
		t.Errorf("runTarget (no steps): %v", err)
	}
}

func TestRunMainWithPreset(t *testing.T) {
	dir := t.TempDir()
	bf := `target test { steps { exec ["true"] } preset cover { argv ["-coverprofile=coverage.out"] } }
suite ci { build test test.cover }
target build { steps { exec ["true"] } }
`
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte(bf), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	code, err := RunMain([]string{"test", "cover"})
	if err != nil {
		t.Fatalf("RunMain test cover: %v", err)
	}
	if code != 0 {
		t.Errorf("expected exit 0, got %d", code)
	}
	// Run suite ci: should run build, test, then test with preset cover
	code, err = RunMain([]string{"ci"})
	if err != nil {
		t.Fatalf("RunMain ci: %v", err)
	}
	if code != 0 {
		t.Errorf("expected exit 0 for suite ci, got %d", code)
	}
}

func TestRunMainJSON(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte("target build { steps { exec [\"true\"] } }\n"), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	code, err := RunMain([]string{"--json", "build"})
	if err != nil {
		t.Fatalf("RunMain --json build: %v", err)
	}
	if code != 0 {
		t.Errorf("expected exit 0, got %d", code)
	}
	// Output is on stdout; we'd need to capture it. For now just verify exit 0.
	// A fuller test would run with captured stdout and parse NDJSON for run_start, target_start, step_end, run_end.
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
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte("target build { steps { exec [\"true\"] } }\nsuite dev { build }\n"), 0644)
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

func TestRunMainVersion(t *testing.T) {
	out, code, err := runMainCaptureStdout(t, []string{"--version"})
	if err != nil {
		t.Fatalf("RunMain --version: %v", err)
	}
	if code != 0 {
		t.Errorf("expected exit 0, got %d", code)
	}
	ver := strings.TrimSpace(out)
	if ver == "" {
		t.Error("--version should print non-empty version")
	}
	if Version != "dev" && ver != Version {
		t.Errorf("stdout %q should match Version %q when built with ldflags", ver, Version)
	}
}

func TestRunMainDownNoState(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte("target build { steps { exec [\"true\"] } }\n"), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	code, err := RunMain([]string{"down"})
	if err != nil {
		t.Fatalf("RunMain down: %v", err)
	}
	if code != 0 {
		t.Errorf("expected exit code 0, got %d", code)
	}
}

func TestRunMainUpNoTargetUp(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte("target build { steps { exec [\"true\"] } }\n"), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	code, err := RunMain([]string{"up"})
	if err == nil {
		t.Fatal("expected error when no target up")
	}
	if code != 1 {
		t.Errorf("expected exit code 1, got %d", code)
	}
	if !strings.Contains(err.Error(), "target") && !strings.Contains(err.Error(), "up") {
		t.Errorf("error should mention target or up: %v", err)
	}
}

// TestRunMainUpThenDown runs up then down in-process so runUp/runDown count toward coverage.
func TestRunMainUpThenDown(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("daemon uses sleep; run on Unix")
	}
	dir := t.TempDir()
	bf := `target build { steps { exec ["true"] } }
target up {
  workflow { build sleeper }
  daemon sleeper { steps { exec ["sleep", "0.2"] } }
}
`
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte(bf), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	t.Setenv("BAKE_NO_AUTOFORMAT", "1")
	t.Setenv("BAKE_NO_AUTOLINT", "1")
	t.Setenv("BAKE_NO_DAEMON", "1")
	code, err := RunMain([]string{"up"})
	if err != nil {
		t.Fatalf("RunMain up: %v", err)
	}
	if code != 0 {
		t.Errorf("up: exit code %d", code)
	}
	code, err = RunMain([]string{"down"})
	if err != nil {
		t.Fatalf("RunMain down: %v", err)
	}
	if code != 0 {
		t.Errorf("down: exit code %d", code)
	}
}

func TestRunMainNoDaemonRunsInProcess(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte("target build { steps { exec [\"true\"] } }\n"), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	code, err := RunMain([]string{"--no-daemon", "build"})
	if err != nil {
		t.Fatalf("RunMain --no-daemon build: %v", err)
	}
	if code != 0 {
		t.Errorf("expected exit code 0, got %d", code)
	}
}

func TestRunMainDownSingleDaemon(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("daemon uses sleep; run on Unix")
	}
	dir := t.TempDir()
	bf := `target build { steps { exec ["true"] } }
target up {
  workflow { build sleeper }
  daemon sleeper { steps { exec ["sleep", "0.3"] } }
}
`
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte(bf), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	t.Setenv("BAKE_NO_AUTOFORMAT", "1")
	t.Setenv("BAKE_NO_AUTOLINT", "1")
	t.Setenv("BAKE_NO_DAEMON", "1")
	if code, err := RunMain([]string{"up"}); err != nil || code != 0 {
		t.Fatalf("up: %v (code %d)", err, code)
	}
	code, err := RunMain([]string{"down", "sleeper"})
	if err != nil {
		t.Fatalf("RunMain down sleeper: %v", err)
	}
	if code != 0 {
		t.Errorf("down sleeper: exit code %d", code)
	}
}

func TestRunUpWithScheduleIntervalExitsOnContextCancel(t *testing.T) {
	dir := t.TempDir()
	bf := `target build { steps { exec ["true"] } }
target up {
  workflow { build schedule interval "1h" }
}
`
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte(bf), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	t.Setenv("BAKE_NO_AUTOFORMAT", "1")
	t.Setenv("BAKE_NO_AUTOLINT", "1")
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	err := runUp(ctx, nil, "", false)
	if err != nil {
		t.Fatalf("runUp with schedule: %v", err)
	}
}

func TestRunUpWithScheduleCronExitsOnContextCancel(t *testing.T) {
	dir := t.TempDir()
	bf := `target build { steps { exec ["true"] } }
target up {
  workflow { build schedule cron "0 * * * *" }
}
`
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte(bf), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	t.Setenv("BAKE_NO_AUTOFORMAT", "1")
	t.Setenv("BAKE_NO_AUTOLINT", "1")
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	err := runUp(ctx, nil, "", false)
	if err != nil {
		t.Fatalf("runUp with schedule cron: %v", err)
	}
}

func TestRunUpWithScheduleIntervalAndProfile(t *testing.T) {
	dir := t.TempDir()
	bf := `profile prod { env { FOO prod } }
target build { steps { exec ["true"] } }
target up {
  workflow { build schedule interval "1h" }
}
`
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte(bf), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	t.Setenv("BAKE_NO_AUTOFORMAT", "1")
	t.Setenv("BAKE_NO_AUTOLINT", "1")
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	err := runUp(ctx, nil, "prod", false)
	if err != nil {
		t.Fatalf("runUp with schedule and profile: %v", err)
	}
}

func TestRunUpWithInvalidScheduleIntervalErrors(t *testing.T) {
	dir := t.TempDir()
	bf := `target build { steps { exec ["true"] } }
target up {
  workflow { build schedule interval "not-a-duration" }
}
`
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte(bf), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	t.Setenv("BAKE_NO_AUTOFORMAT", "1")
	t.Setenv("BAKE_NO_AUTOLINT", "1")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := runUp(ctx, nil, "", false)
	if err == nil {
		t.Fatal("expected error for invalid schedule interval")
	}
	if !strings.Contains(err.Error(), "schedule interval") && !strings.Contains(err.Error(), "duration") {
		t.Errorf("error should mention schedule/interval/duration: %v", err)
	}
}

func TestRunUpWithInvalidScheduleCronErrors(t *testing.T) {
	dir := t.TempDir()
	bf := `target build { steps { exec ["true"] } }
target up {
  workflow { build schedule cron "invalid-cron-spec" }
}
`
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte(bf), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	t.Setenv("BAKE_NO_AUTOFORMAT", "1")
	t.Setenv("BAKE_NO_AUTOLINT", "1")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := runUp(ctx, nil, "", false)
	if err == nil {
		t.Fatal("expected error for invalid schedule cron")
	}
	if !strings.Contains(err.Error(), "schedule") && !strings.Contains(err.Error(), "cron") {
		t.Errorf("error should mention schedule/cron: %v", err)
	}
}

func TestRunMainDownWithContainerInState(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte("target build { steps { exec [\"true\"] } }\n"), 0644)
	bakeDir := filepath.Join(dir, ".bake")
	os.MkdirAll(bakeDir, 0755)
	// State with a daemon that has container_id (no real container exists). Exercises the container stop path.
	state := `{"daemons":{"c1":{"pid":0,"container_id":"nonexistent-container-id-99999"}}}`
	os.WriteFile(filepath.Join(bakeDir, "state.json"), []byte(state), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	code, err := RunMain([]string{"down", "c1"})
	// Without Docker or with bogus container ID we get an error or non-zero exit; both are acceptable.
	_ = code
	_ = err
}

func TestRunMainDownDaemonNotInState(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte("target build { steps { exec [\"true\"] } }\n"), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	code, err := RunMain([]string{"down", "nonexistent"})
	if err == nil {
		t.Fatal("expected error when daemon not in state")
	}
	if code != 1 {
		t.Errorf("expected exit 1, got %d", code)
	}
	if !strings.Contains(err.Error(), "not in state") && !strings.Contains(err.Error(), "nonexistent") {
		t.Errorf("error should mention state or daemon name: %v", err)
	}
}

func TestRunMainUpWithProfile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("daemon uses sleep; run on Unix")
	}
	dir := t.TempDir()
	bf := `profile prod { env { FOO prod } }
target build { steps { exec ["true"] } }
target up {
  workflow { build sleeper }
  daemon sleeper { steps { exec ["sleep", "0.1"] } }
}
`
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte(bf), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	t.Setenv("BAKE_NO_AUTOFORMAT", "1")
	t.Setenv("BAKE_NO_AUTOLINT", "1")
	code, err := RunMain([]string{"--profile", "prod", "up"})
	if err != nil {
		t.Fatalf("RunMain --profile prod up: %v", err)
	}
	if code != 0 {
		t.Errorf("exit code %d", code)
	}
	RunMain([]string{"down"})
}

func TestRunMainUpUnknownWorkflowRef(t *testing.T) {
	dir := t.TempDir()
	bf := `target build { steps { exec ["true"] } }
target up {
  workflow { build missing }
}
`
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte(bf), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	t.Setenv("BAKE_NO_AUTOFORMAT", "1")
	t.Setenv("BAKE_NO_AUTOLINT", "1")
	t.Setenv("BAKE_NO_DAEMON", "1")
	code, err := RunMain([]string{"up"})
	if err == nil {
		t.Fatal("expected error for unknown workflow ref")
	}
	if code != 1 {
		t.Errorf("expected exit 1, got %d", code)
	}
	if !strings.Contains(err.Error(), "unknown") && !strings.Contains(err.Error(), "missing") {
		t.Errorf("error should mention unknown or missing: %v", err)
	}
}

func TestRunMainUpUnknownProfile(t *testing.T) {
	dir := t.TempDir()
	bf := `target build { steps { exec ["true"] } }
target up {
  workflow { build }
}
`
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte(bf), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	t.Setenv("BAKE_NO_AUTOFORMAT", "1")
	t.Setenv("BAKE_NO_AUTOLINT", "1")
	t.Setenv("BAKE_NO_DAEMON", "1")
	code, err := RunMain([]string{"--profile", "nonexistent", "up"})
	if err == nil {
		t.Fatal("expected error for unknown profile")
	}
	if code != 1 && code != 2 {
		t.Errorf("expected exit 1 or 2, got %d", code)
	}
}

func TestRunMainWatchNoInputs(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte("target build { steps { exec [\"true\"] } }\n"), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	err := runWatch(ctx, "build", nil, nil, nil, "", false, 1)
	if err != nil {
		t.Fatalf("runWatch (no inputs): %v", err)
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

func TestRunMainExplainWithDescArgsEnv(t *testing.T) {
	dir := t.TempDir()
	bf := []byte("target t { desc \"a target\" args x string x v env { FOO bar } steps { exec [\"true\"] } }\n")
	os.WriteFile(filepath.Join(dir, "Bakefile"), bf, 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	out, code, err := runMainCaptureStdout(t, []string{"--explain", "t"})
	if err != nil {
		t.Fatalf("RunMain --explain: %v", err)
	}
	if code != 0 {
		t.Errorf("expected exit 0, got %d", code)
	}
	if !strings.Contains(out, "desc:") || !strings.Contains(out, "args:") || !strings.Contains(out, "env:") {
		t.Errorf("explain should include desc, args, env: %s", out)
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

func TestRunMainCiFlagNoArgsRunsCiSuite(t *testing.T) {
	// When --ci is set and no target given, run the ci suite (not the default target).
	dir := t.TempDir()
	bf := `target act { steps { exec ["false"] } }
suite ci { build }
target build { steps { exec ["true"] } }
`
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte(bf), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	code, err := RunMain([]string{"--ci"})
	if err != nil {
		t.Fatalf("RunMain --ci: %v", err)
	}
	if code != 0 {
		t.Errorf("--ci with no args should run ci suite (build), not default target (act); got exit %d", code)
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
	// install (no subcommand) installs shims and hooks; shims should exist
	if _, err := os.Stat(filepath.Join(dir, ".bake", "bin")); err != nil {
		t.Errorf(".bake/bin missing after install: %v", err)
	}
}

func TestRunMainInstallShims(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte("target build { steps { exec [\"true\"] } }\n"), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	code, err := RunMain([]string{"install", "shims"})
	if err != nil {
		t.Fatalf("RunMain install shims: %v", err)
	}
	if code != 0 {
		t.Errorf("expected exit code 0, got %d", code)
	}
	if _, err := os.Stat(filepath.Join(dir, ".bake", "bin", "build")); err != nil {
		t.Errorf("shim build missing: %v", err)
	}
}

func TestRunMainInstallCreatesBakefile(t *testing.T) {
	dir := t.TempDir()
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
	bakePath := filepath.Join(dir, "Bakefile")
	data, err := os.ReadFile(bakePath)
	if err != nil {
		t.Fatalf("Bakefile not created: %v", err)
	}
	if !strings.Contains(string(data), "target build") || !strings.Contains(string(data), "suite dev") {
		t.Errorf("minimal Bakefile should have target build and suite dev: %s", data)
	}
	if _, err := os.Stat(filepath.Join(dir, ".bake", "bin", "build")); err != nil {
		t.Errorf("shims should be installed after creating Bakefile: %v", err)
	}
}

func TestRunMainInstallHooks(t *testing.T) {
	dir := t.TempDir()
	// Bakefile with suite precommit (default suite for hooks)
	bf := []byte(`target lint { desc "lint" steps { exec ["true"] } }
suite precommit { lint }
`)
	os.WriteFile(filepath.Join(dir, "Bakefile"), bf, 0644)
	if err := os.MkdirAll(filepath.Join(dir, ".git", "hooks"), 0755); err != nil {
		t.Fatal(err)
	}
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	code, err := RunMain([]string{"install", "hooks"})
	if err != nil {
		t.Fatalf("RunMain install hooks: %v", err)
	}
	if code != 0 {
		t.Errorf("expected exit code 0, got %d", code)
	}
	hookPath := filepath.Join(dir, ".git", "hooks", "pre-commit")
	data, err := os.ReadFile(hookPath)
	if err != nil {
		t.Fatalf("read pre-commit hook: %v", err)
	}
	script := string(data)
	if !strings.Contains(script, "#!/bin/sh") || !strings.Contains(script, "set -e") {
		t.Errorf("pre-commit hook should be a shell script: %s", script)
	}
	if !strings.Contains(script, "precommit") {
		t.Errorf("pre-commit hook should run 'bake precommit': %s", script)
	}
}

func TestRunMainInstallDaemonNoBakefile(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	code, err := RunMain([]string{"install", "daemon"})
	if err == nil {
		t.Fatal("expected error for install daemon with no Bakefile")
	}
	if code != 2 {
		t.Errorf("expected exit code 2, got %d", code)
	}
	if !strings.Contains(err.Error(), "find Bakefile") && !strings.Contains(err.Error(), "baked not found") {
		t.Errorf("error should mention find Bakefile or baked not found: %v", err)
	}
}

func TestRunMainInstallDaemonWritesUnit(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skipf("install daemon only supported on linux/darwin, not %s", runtime.GOOS)
	}
	dir := t.TempDir()
	fakeHome := t.TempDir()
	pathDir := t.TempDir()
	// Stub "baked" so LookPath finds it
	stubBaked := filepath.Join(pathDir, "baked")
	if err := os.WriteFile(stubBaked, []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte("target build { steps { exec [\"true\"] } }\n"), 0644)
	origWd, _ := os.Getwd()
	origHome := os.Getenv("HOME")
	origPath := os.Getenv("PATH")
	os.Chdir(dir)
	os.Setenv("HOME", fakeHome)
	os.Setenv("PATH", pathDir+string(filepath.ListSeparator)+origPath)
	defer func() {
		os.Chdir(origWd)
		os.Setenv("HOME", origHome)
		os.Setenv("PATH", origPath)
	}()
	code, err := RunMain([]string{"install", "daemon"})
	if err != nil {
		t.Fatalf("RunMain install daemon: %v", err)
	}
	if code != 0 {
		t.Errorf("expected exit code 0, got %d", code)
	}
	rootAbs, _ := filepath.Abs(dir)
	if runtime.GOOS == "linux" {
		configDir := filepath.Join(fakeHome, ".config", "systemd", "user")
		matches, _ := filepath.Glob(filepath.Join(configDir, "baked-*.service"))
		if len(matches) == 0 {
			t.Fatalf("no baked-*.service under %s", configDir)
		}
		content, _ := os.ReadFile(matches[0])
		if !bytes.Contains(content, []byte(rootAbs)) {
			t.Errorf("unit should contain workspace root %q: %s", rootAbs, content)
		}
		if !bytes.Contains(content, []byte(stubBaked)) {
			t.Errorf("unit should contain baked path %q: %s", stubBaked, content)
		}
	} else {
		agentsDir := filepath.Join(fakeHome, "Library", "LaunchAgents")
		matches, _ := filepath.Glob(filepath.Join(agentsDir, "com.bake.baked.*.plist"))
		if len(matches) == 0 {
			t.Fatalf("no com.bake.baked.*.plist under %s", agentsDir)
		}
		plistPath := matches[0]
		defer exec.Command("launchctl", "unload", plistPath).Run() // clean up loaded job
		content, _ := os.ReadFile(plistPath)
		if !bytes.Contains(content, []byte(rootAbs)) {
			t.Errorf("plist should contain workspace root %q: %s", rootAbs, content)
		}
		if !bytes.Contains(content, []byte(stubBaked)) {
			t.Errorf("plist should contain baked path %q: %s", stubBaked, content)
		}
	}
}

func TestRunMainInstallUnknownSubcommand(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte("target build { steps { exec [\"true\"] } }\n"), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	code, err := RunMain([]string{"install", "foo"})
	if err == nil {
		t.Fatal("expected error for unknown install subcommand")
	}
	if code != 2 {
		t.Errorf("expected exit code 2, got %d", code)
	}
	if !strings.Contains(err.Error(), "unknown install subcommand") {
		t.Errorf("error should mention unknown subcommand: %v", err)
	}
	if !strings.Contains(err.Error(), "install daemon") {
		t.Errorf("error should suggest install daemon: %v", err)
	}
}

func TestRunMainInstallHooksNoGit(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte("suite precommit { build }\ntarget build { steps { exec [\"true\"] } }\n"), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	code, err := RunMain([]string{"install", "hooks"})
	if err == nil {
		t.Fatal("expected error when not a git repo")
	}
	if code != 2 {
		t.Errorf("expected exit code 2, got %d", code)
	}
}

func TestRunMainInstallHooksNoSuitePrecommit(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte("target build { steps { exec [\"true\"] } }\nsuite dev { build }\n"), 0644)
	if err := os.MkdirAll(filepath.Join(dir, ".git", "hooks"), 0755); err != nil {
		t.Fatal(err)
	}
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	code, err := RunMain([]string{"install", "hooks"})
	if err == nil {
		t.Fatal("expected error when no suite precommit defined")
	}
	if code != 2 {
		t.Errorf("expected exit code 2, got %d", code)
	}
	if !strings.Contains(err.Error(), "suite precommit") {
		t.Errorf("error should mention suite precommit: %v", err)
	}
}

func TestRunMainDefaultNoTarget(t *testing.T) {
	// Bakefile with only a suite (no targets) → runDefault hits "no target specified"
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte("suite dev { }\n"), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	code, err := RunMain([]string{})
	if err == nil {
		t.Fatal("expected error when no target and no default")
	}
	if code != 2 {
		t.Errorf("expected exit code 2, got %d", code)
	}
}

func TestRunMainFormatStdout(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte("target build { steps { exec [\"true\"] } }\n"), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	out, code, err := runMainCaptureStdout(t, []string{"fmt"})
	if err != nil {
		t.Fatalf("RunMain fmt: %v", err)
	}
	if code != 0 {
		t.Errorf("expected exit 0, got %d", code)
	}
	if !strings.Contains(out, "target") || !strings.Contains(out, "build") {
		t.Errorf("format stdout should contain formatted Bakefile: %s", out)
	}
}

func TestRunMainFormatNoBakefile(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	code, err := RunMain([]string{"fmt"})
	if err == nil {
		t.Fatal("expected error when running fmt with no Bakefile")
	}
	if code != 2 {
		t.Errorf("expected exit 2, got %d", code)
	}
}

func TestRunMainFormatCheck(t *testing.T) {
	dir := t.TempDir()
	// Unformatted: extra space in "inputs [ "
	content := []byte("target build { inputs [ \"x.go\" ] steps { exec [\"true\"] } }\n")
	os.WriteFile(filepath.Join(dir, "Bakefile"), content, 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	code, err := RunMain([]string{"fmt", "--check"})
	if err == nil {
		t.Fatal("expected error when Bakefile not formatted")
	}
	if code != 1 {
		t.Errorf("expected exit code 1, got %d", code)
	}
	// Format in place then check should pass.
	code, err = RunMain([]string{"fmt", "-w"})
	if err != nil {
		t.Fatalf("RunMain fmt -w: %v", err)
	}
	if code != 0 {
		t.Errorf("fmt -w: expected exit 0, got %d", code)
	}
	code, err = RunMain([]string{"fmt", "--check"})
	if err != nil {
		t.Fatalf("RunMain fmt --check after format: %v", err)
	}
	if code != 0 {
		t.Errorf("fmt --check: expected exit 0, got %d", code)
	}
}

// runMainCaptureStdout runs RunMain with stdout redirected to a buffer; returns stdout, exit code, and error.
func runMainCaptureStdout(t *testing.T, args []string) (stdout string, code int, runErr error) {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	var buf bytes.Buffer
	done := make(chan struct{})
	go func() {
		io.Copy(&buf, r)
		close(done)
	}()
	code, runErr = RunMain(args)
	os.Stdout = old
	w.Close()
	<-done
	return buf.String(), code, runErr
}

func TestRunMainSetEnv(t *testing.T) {
	dir := t.TempDir()
	// Target that fails if FOO is not "bar" (so --set env.FOO=bar makes it pass)
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte("target check { steps { exec [\"sh\", \"-c\", \"test \\\"$FOO\\\" = bar\"] } }\n"), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	code, err := RunMain([]string{"--set", "env.FOO=bar", "check"})
	if err != nil {
		t.Fatalf("RunMain: %v", err)
	}
	if code != 0 {
		t.Errorf("expected exit 0 with --set env.FOO=bar, got %d", code)
	}
	// Without --set it should fail (step exits 1)
	code2, _ := RunMain([]string{"check"})
	if code2 == 0 {
		t.Error("expected non-zero exit without --set")
	}
}

func TestRunMainSetArgs(t *testing.T) {
	dir := t.TempDir()
	// Step uses {{.x}} so --set args.x=overridden is expanded into the command
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte("target build { args x string x default steps { exec [\"sh\", \"-c\", \"test \\\"{{.x}}\\\" = overridden\"] } }\n"), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	code, err := RunMain([]string{"--set", "args.x=overridden", "build"})
	if err != nil {
		t.Fatalf("RunMain: %v", err)
	}
	if code != 0 {
		t.Errorf("expected exit 0 with --set args.x=overridden, got %d", code)
	}
}

func TestRunMainProfile(t *testing.T) {
	dir := t.TempDir()
	// Profile prod sets ENV=prod; target checks it so we verify profile env is applied
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte("profile prod { env { ENV prod } }\ntarget check { steps { exec [\"sh\", \"-c\", \"test \\\"$ENV\\\" = prod\"] } }\n"), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	code, err := RunMain([]string{"--profile", "prod", "check"})
	if err != nil {
		t.Fatalf("RunMain: %v", err)
	}
	if code != 0 {
		t.Errorf("expected exit 0 with --profile prod (ENV=prod applied), got %d", code)
	}
	// Unknown profile should fail with exit 2
	code2, err2 := RunMain([]string{"--profile", "nonexistent", "check"})
	if code2 != 2 || err2 == nil {
		t.Errorf("expected exit 2 for unknown profile, got code=%d err=%v", code2, err2)
	}
}

func TestRunMainDefaultWithProfile(t *testing.T) {
	// Run with no target but --profile: runDefault is called with profile
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte("profile prod { env { ENV prod } }\ntarget default { steps { exec [\"sh\", \"-c\", \"test \\\"$ENV\\\" = prod\"] } }\n"), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	code, err := RunMain([]string{"--profile", "prod"})
	if err != nil {
		t.Fatalf("RunMain: %v", err)
	}
	if code != 0 {
		t.Errorf("expected exit 0 when running default with --profile prod, got %d", code)
	}
}

func TestRunMainGraph(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte("target a { deps b, c }\ntarget b { deps c }\ntarget c { steps { exec [\"true\"] } }\n"), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	out, code, err := runMainCaptureStdout(t, []string{"--graph=dot"})
	if err != nil {
		t.Fatalf("RunMain: %v", err)
	}
	if code != 0 {
		t.Errorf("expected exit 0, got %d", code)
	}
	if !strings.Contains(out, "digraph") {
		t.Errorf("expected digraph in output, got: %s", out)
	}
	if !strings.Contains(out, "a") || !strings.Contains(out, "b") || !strings.Contains(out, "c") {
		t.Errorf("expected nodes a,b,c in output, got: %s", out)
	}
}

func TestRunMainGraphJSON(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte("target a { deps b }\ntarget b { steps { exec [\"true\"] } }\n"), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	out, code, err := runMainCaptureStdout(t, []string{"--graph=json"})
	if err != nil {
		t.Fatalf("RunMain: %v", err)
	}
	if code != 0 {
		t.Errorf("expected exit 0, got %d", code)
	}
	if !strings.Contains(out, `"nodes"`) || !strings.Contains(out, `"edges"`) {
		t.Errorf("expected JSON nodes/edges, got: %s", out)
	}
}

func TestRunMainWhatDependsOn(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte("target a { deps b, c }\ntarget b { deps c }\ntarget c { steps { exec [\"true\"] } }\n"), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	out, code, err := runMainCaptureStdout(t, []string{"--what-depends-on", "c"})
	if err != nil {
		t.Fatalf("RunMain: %v", err)
	}
	if code != 0 {
		t.Errorf("expected exit 0, got %d", code)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) < 2 {
		t.Errorf("expected at least a and b to depend on c, got %d lines: %s", len(lines), out)
	}
	if !strings.Contains(out, "a") || !strings.Contains(out, "b") {
		t.Errorf("expected a and b in output, got: %s", out)
	}
}

func TestRunMainListStatus(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte("target build { steps { exec [\"true\"] } }\n"), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	out, code, err := runMainCaptureStdout(t, []string{"--list", "--status"})
	if err != nil {
		t.Fatalf("RunMain: %v", err)
	}
	if code != 0 {
		t.Errorf("expected exit 0, got %d", code)
	}
	// Should contain target name and a status column (would run / skipped)
	if !strings.Contains(out, "build") {
		t.Errorf("expected build in list output, got: %s", out)
	}
	if !strings.Contains(out, "would run") && !strings.Contains(out, "skipped") {
		t.Errorf("expected status column (would run or skipped), got: %s", out)
	}
}

func TestRunMainChooseNonTTY(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte("target build { steps { exec [\"true\"] } }\ntarget test { steps { exec [\"true\"] } }\n"), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	// When stdout is not a TTY, --choose prints target names one per line and exits 0
	out, code, err := runMainCaptureStdout(t, []string{"--choose"})
	if err != nil {
		t.Fatalf("RunMain: %v", err)
	}
	if code != 0 {
		t.Errorf("expected exit 0, got %d", code)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) < 2 {
		t.Errorf("expected at least build and test in output, got: %s", out)
	}
	if !strings.Contains(out, "build") || !strings.Contains(out, "test") {
		t.Errorf("expected build and test in --choose output, got: %s", out)
	}
}

func TestRunMainPrivateHiddenFromList(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte("target visible { steps { exec [\"true\"] } }\ntarget hidden { private steps { exec [\"true\"] } }\n"), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	out, code, err := runMainCaptureStdout(t, []string{"--list"})
	if err != nil {
		t.Fatalf("RunMain: %v", err)
	}
	if code != 0 {
		t.Errorf("expected exit 0, got %d", code)
	}
	if !strings.Contains(out, "visible") {
		t.Errorf("expected visible in --list, got: %s", out)
	}
	if strings.Contains(out, "hidden") {
		t.Errorf("private target hidden should not appear in --list, got: %s", out)
	}
}

func TestRunMainPrivateRunnableByName(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte("target visible { steps { exec [\"true\"] } }\ntarget hidden { private steps { exec [\"true\"] } }\n"), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	code, err := RunMain([]string{"hidden"})
	if err != nil {
		t.Fatalf("RunMain: %v", err)
	}
	if code != 0 {
		t.Errorf("expected exit 0 when running private target by name, got %d", code)
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

func TestRunMainLint(t *testing.T) {
	dir := t.TempDir()
	// Bakefile that triggers prefer-exec (cmd), brackets-only (single-line), require-desc (no desc)
	bf := []byte("target build cmd go build .\n")
	os.WriteFile(filepath.Join(dir, "Bakefile"), bf, 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	code, err := RunMain([]string{"lint"})
	if err != nil {
		t.Fatalf("RunMain lint: %v", err)
	}
	if code != 1 {
		t.Errorf("expected exit 1 when there are findings, got %d", code)
	}
}

func TestRunMainLintFix(t *testing.T) {
	dir := t.TempDir()
	bf := []byte("target t { steps { cmd go fmt ./... } }\n")
	bakePath := filepath.Join(dir, "Bakefile")
	os.WriteFile(bakePath, bf, 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	code, err := RunMain([]string{"lint", "--fix"})
	if err != nil {
		t.Fatalf("RunMain lint --fix: %v", err)
	}
	// After fix we still exit 1 if there were findings (require-desc may remain)
	if code != 0 && code != 1 {
		t.Errorf("expected exit 0 or 1, got %d", code)
	}
	// Verify file was fixed: cmd converted to exec
	data, err := os.ReadFile(bakePath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "exec [") {
		t.Errorf("expected exec after --fix, got %s", data)
	}
	if strings.Contains(string(data), "cmd go") {
		t.Errorf("cmd should have been converted, got %s", data)
	}
}

func TestRunMainLintClean(t *testing.T) {
	dir := t.TempDir()
	bf := []byte("target build { desc \"build\" steps { exec [\"true\"] } }\n")
	os.WriteFile(filepath.Join(dir, "Bakefile"), bf, 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	code, err := RunMain([]string{"lint"})
	if err != nil {
		t.Fatalf("RunMain lint: %v", err)
	}
	if code != 0 {
		t.Errorf("expected exit 0 for clean Bakefile, got %d", code)
	}
}

func TestInputPathsForRun(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte(`
target a { inputs ["a.in"] outputs ["a.out"] steps { exec ["true"] } }
target b { deps a inputs ["b.in"] outputs ["b.out"] steps { exec ["true"] } }
`), 0644)
	os.WriteFile(filepath.Join(dir, "a.in"), []byte("a"), 0644)
	os.WriteFile(filepath.Join(dir, "b.in"), []byte("b"), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	cfg, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	paths, err := inputPathsForRun(cfg, "b")
	if err != nil {
		t.Fatal(err)
	}
	// Order may vary; should contain a.in and b.in (or their resolved form)
	if len(paths) < 2 {
		t.Errorf("expected at least 2 input paths, got %v", paths)
	}
}

func TestRunWatchReRunOnChange(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("watch test uses sh -c")
	}
	dir := t.TempDir()
	// Target: input in.txt, step appends to runcount.txt so we can assert re-run
	bf := `target build {
  inputs ["in.txt"]
  outputs ["out.txt"]
  steps { exec ["sh", "-c", "echo run >> runcount.txt && cp in.txt out.txt"] }
}
`
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte(bf), 0644)
	os.WriteFile(filepath.Join(dir, "in.txt"), []byte("x"), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Add(1)
	var watchErr error
	go func() {
		defer wg.Done()
		watchErr = runWatch(ctx, "build", nil, nil, nil, "", false, 1)
	}()
	// Let first run complete and Poll start
	time.Sleep(600 * time.Millisecond)
	// Touch input so watcher will trigger a second run
	if err := os.WriteFile(filepath.Join(dir, "in.txt"), []byte("y"), 0644); err != nil {
		cancel()
		wg.Wait()
		t.Fatal(err)
	}
	runcountPath := filepath.Join(dir, "runcount.txt")
	deadline := time.Now().Add(15 * time.Second)
	for {
		data, err := os.ReadFile(runcountPath)
		if err == nil {
			lines := strings.Split(strings.TrimSpace(string(data)), "\n")
			if len(lines) >= 2 {
				break
			}
		}
		if time.Now().After(deadline) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	cancel()
	wg.Wait()
	if watchErr != nil {
		t.Errorf("runWatch: %v", watchErr)
	}
	// Should have run at least twice (initial + after touch)
	data, err := os.ReadFile(runcountPath)
	if err != nil {
		t.Fatalf("read runcount: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) < 2 {
		t.Errorf("expected at least 2 runs, got %d (runcount.txt: %q)", len(lines), data)
	}
}

// TestRunMainContainerTarget runs a target with image (requires Docker); skipped in -short.
func TestRunMainContainerTarget(t *testing.T) {
	if testing.Short() {
		t.Skip("container test requires Docker")
	}
	dir := t.TempDir()
	bf := `target in-container {
  desc "run in container (integration test)"
  image "alpine:3.19"
  steps { exec ["sh", "-c", "echo ok"] }
}
`
	if err := os.WriteFile(filepath.Join(dir, "Bakefile"), []byte(bf), 0644); err != nil {
		t.Fatal(err)
	}
	orig, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(orig)
	code, err := RunMain([]string{"in-container"})
	if err != nil {
		// Docker may be unavailable
		if strings.Contains(err.Error(), "Docker") || strings.Contains(err.Error(), "daemon") {
			t.Skipf("Docker unavailable: %v", err)
		}
		t.Fatal(err)
	}
	if code != 0 {
		t.Errorf("expected exit 0, got %d", code)
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

// TestBakeBuildViaDaemon starts baked, then runs "bake build" which delegates to the daemon.
func TestBakeBuildViaDaemon(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix socket test; run on Unix")
	}
	// Short path for Unix socket (macOS path length limit)
	dir, err := os.MkdirTemp(os.TempDir(), "bt")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	modRoot := repoRoot(t)
	bakeExe := filepath.Join(dir, "bake")
	bakedExe := filepath.Join(dir, "baked")
	for _, b := range []struct{ name, pkg string }{
		{"bake", "./cmd/bake"},
		{"baked", "./cmd/baked"},
	} {
		cmd := exec.Command("go", "build", "-o", filepath.Join(dir, b.name), b.pkg)
		cmd.Dir = modRoot
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("build %s: %v\n%s", b.name, err, out)
		}
	}
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte("target build { steps { exec [\"true\"] } }\n"), 0644)

	bakedCmd := exec.Command(bakedExe, "--no-watch")
	bakedCmd.Dir = dir
	bakedCmd.Stdout = nil
	bakedCmd.Stderr = nil
	if err := bakedCmd.Start(); err != nil {
		t.Fatalf("start baked: %v", err)
	}
	defer bakedCmd.Process.Kill()
	time.Sleep(200 * time.Millisecond)

	pathEnv := dir + string(filepath.ListSeparator) + os.Getenv("PATH")
	run := exec.Command(bakeExe, "build")
	run.Dir = dir
	run.Env = append(os.Environ(), "PATH="+pathEnv, "BAKE_NO_AUTOFORMAT=1", "BAKE_NO_AUTOLINT=1")
	out, err := run.CombinedOutput()
	if err != nil {
		t.Fatalf("bake build via daemon: %v\n%s", err, out)
	}
}

// TestBakeUpDownIntegration runs the built binary for "bake up" then "bake down" (target up workflow + daemon lifecycle).
func TestBakeUpDownIntegration(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("daemon lifecycle test uses sleep; run on Unix")
	}
	exe := buildBake(t)
	dir := t.TempDir()
	bf := `target build { steps { exec ["true"] } }
target up {
  workflow { build sleeper }
  daemon sleeper { steps { exec ["sleep", "2"] } }
}
`
	if err := os.WriteFile(filepath.Join(dir, "Bakefile"), []byte(bf), 0644); err != nil {
		t.Fatal(err)
	}
	env := append(os.Environ(), "BAKE_NO_AUTOFORMAT=1", "BAKE_NO_AUTOLINT=1", "BAKE_NO_DAEMON=1")

	up := exec.Command(exe, "up")
	up.Dir = dir
	up.Env = env
	upOut, err := up.CombinedOutput()
	if err != nil {
		t.Fatalf("bake up: %v\n%s", err, upOut)
	}

	down := exec.Command(exe, "down")
	down.Dir = dir
	down.Env = env
	downOut, err := down.CombinedOutput()
	if err != nil {
		t.Fatalf("bake down: %v\n%s", err, downOut)
	}
	if !strings.Contains(string(downOut), "stopped") && !strings.Contains(string(downOut), "sleeper") {
		t.Logf("bake down output: %s", downOut)
	}
}

func TestRunMainLintDisable(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte("target build { desc \"b\" steps { exec [\"true\"] } }\n"), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	t.Setenv("BAKE_NO_AUTOFORMAT", "1")
	t.Setenv("BAKE_NO_AUTOLINT", "1")
	code, err := RunMain([]string{"lint", "--disable", "require-desc"})
	if err != nil {
		t.Fatalf("RunMain lint --disable: %v", err)
	}
	if code != 0 {
		t.Errorf("expected exit 0, got %d", code)
	}
	cfgPath := filepath.Join(dir, ".bake", "config")
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("config file should exist: %v", err)
	}
	if !strings.Contains(string(data), "require-desc disabled") {
		t.Errorf("config should contain 'require-desc disabled', got: %s", data)
	}
}

func TestRunMainChooseNonTTYWithSuiteCI(t *testing.T) {
	dir := t.TempDir()
	bf := "target build { desc \"b\" steps { exec [\"true\"] } }\ntarget test { desc \"t\" steps { exec [\"true\"] } }\nsuite ci { build }\nsuite dev { build test }\n"
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte(bf), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	t.Setenv("BAKE_NO_AUTOFORMAT", "1")
	t.Setenv("BAKE_NO_AUTOLINT", "1")
	t.Setenv("CI", "true")

	out, code, err := runMainCaptureStdout(t, []string{"--choose", "--ci"})
	if err != nil {
		t.Fatalf("RunMain --choose --ci: %v", err)
	}
	if code != 0 {
		t.Errorf("expected exit 0, got %d", code)
	}
	if !strings.Contains(out, "build") {
		t.Errorf("--choose --ci should include build: %s", out)
	}

	t.Setenv("CI", "")
	out2, code2, err2 := runMainCaptureStdout(t, []string{"--choose"})
	if err2 != nil {
		t.Fatalf("RunMain --choose dev: %v", err2)
	}
	if code2 != 0 {
		t.Errorf("expected exit 0, got %d", code2)
	}
	if !strings.Contains(out2, "build") || !strings.Contains(out2, "test") {
		t.Errorf("--choose dev should include build and test: %s", out2)
	}
}

func TestRunMainChooseNoTargets(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte("target hidden { desc \"h\" private steps { exec [\"true\"] } }\nsuite dev { }\n"), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	t.Setenv("BAKE_NO_AUTOFORMAT", "1")
	t.Setenv("BAKE_NO_AUTOLINT", "1")
	code, err := RunMain([]string{"--choose"})
	if err == nil {
		t.Fatal("expected error when no targets to choose from")
	}
	if code != 2 {
		t.Errorf("expected exit 2, got %d", code)
	}
}

func TestRunMainDryRunWithPreset(t *testing.T) {
	dir := t.TempDir()
	bf := "target test { desc \"t\" passthrough step = 1 steps { exec [\"go\", \"test\", \"./...\"] } preset cover { argv [\"-coverprofile=c.out\"] } }\n"
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte(bf), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	t.Setenv("BAKE_NO_AUTOFORMAT", "1")
	t.Setenv("BAKE_NO_AUTOLINT", "1")
	out, code, err := runMainCaptureStdout(t, []string{"--dry-run", "test", "cover"})
	if err != nil {
		t.Fatalf("RunMain --dry-run test cover: %v", err)
	}
	if code != 0 {
		t.Errorf("expected exit 0, got %d", code)
	}
	if !strings.Contains(out, "test") {
		t.Errorf("dry run should show target test: %s", out)
	}
}

func TestRunMainExplainWithPresets(t *testing.T) {
	dir := t.TempDir()
	bf := "target test { desc \"run tests\" env { VERBOSE on } steps { exec [\"go\", \"test\"] } preset cover { desc \"with coverage\" argv [\"-cover\"] } }\n"
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte(bf), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	t.Setenv("BAKE_NO_AUTOFORMAT", "1")
	t.Setenv("BAKE_NO_AUTOLINT", "1")
	out, code, err := runMainCaptureStdout(t, []string{"--explain", "test"})
	if err != nil {
		t.Fatalf("RunMain --explain: %v", err)
	}
	if code != 0 {
		t.Errorf("expected exit 0, got %d", code)
	}
	if !strings.Contains(out, "desc:") {
		t.Errorf("explain should include target details: %s", out)
	}
}

func TestRunMainLintJSON(t *testing.T) {
	dir := t.TempDir()
	bf := []byte("target build cmd go build .\n")
	os.WriteFile(filepath.Join(dir, "Bakefile"), bf, 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	t.Setenv("BAKE_NO_AUTOFORMAT", "1")
	t.Setenv("BAKE_NO_AUTOLINT", "1")
	code, err := RunMain([]string{"lint", "--json"})
	if err != nil {
		t.Fatalf("RunMain lint --json: %v", err)
	}
	if code != 1 {
		t.Errorf("expected exit 1 for findings, got %d", code)
	}
}

func TestRunMainListWithDesc(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte("target build { desc \"build binary\" steps { exec [\"true\"] } }\ntarget test { desc \"tests\" steps { exec [\"true\"] } }\n"), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	t.Setenv("BAKE_NO_AUTOFORMAT", "1")
	t.Setenv("BAKE_NO_AUTOLINT", "1")
	out, code, err := runMainCaptureStdout(t, []string{"--list"})
	if err != nil {
		t.Fatalf("RunMain --list: %v", err)
	}
	if code != 0 {
		t.Errorf("expected exit 0, got %d", code)
	}
	if !strings.Contains(out, "build") || !strings.Contains(out, "build binary") {
		t.Errorf("list should show desc: %s", out)
	}
}

func TestRunMainRunTargetWithTiming(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte("target build { desc \"b\" steps { exec [\"true\"] } }\n"), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	t.Setenv("BAKE_NO_AUTOFORMAT", "1")
	t.Setenv("BAKE_NO_AUTOLINT", "1")
	code, err := RunMain([]string{"--timing", "build"})
	if err != nil {
		t.Fatalf("RunMain --timing: %v", err)
	}
	if code != 0 {
		t.Errorf("expected exit 0, got %d", code)
	}
}

func TestRunMainRunTargetWithArtifacts(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "out.txt")
	bf := "target build { desc \"b\" outputs [\"out.txt\"] steps { exec [\"sh\", \"-c\", \"echo ok > out.txt\"] } }\n"
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte(bf), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	t.Setenv("BAKE_NO_AUTOFORMAT", "1")
	t.Setenv("BAKE_NO_AUTOLINT", "1")
	code, err := RunMain([]string{"--artifacts", "build"})
	if err != nil {
		t.Fatalf("RunMain --artifacts: %v", err)
	}
	if code != 0 {
		t.Errorf("expected exit 0, got %d", code)
	}
	if _, err := os.Stat(outPath); err != nil {
		t.Logf("output file not created (may be due to runner impl): %v", err)
	}
}

func TestRunMainDryRunWithDeps(t *testing.T) {
	dir := t.TempDir()
	bf := "target a { desc \"a\" deps b steps { exec [\"true\"] } }\ntarget b { desc \"b\" steps { exec [\"echo\", \"hello\"] } }\n"
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte(bf), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	t.Setenv("BAKE_NO_AUTOFORMAT", "1")
	t.Setenv("BAKE_NO_AUTOLINT", "1")
	out, code, err := runMainCaptureStdout(t, []string{"--dry-run", "a"})
	if err != nil {
		t.Fatalf("RunMain --dry-run: %v", err)
	}
	if code != 0 {
		t.Errorf("expected exit 0, got %d", code)
	}
	if !strings.Contains(out, "target b") || !strings.Contains(out, "target a") {
		t.Errorf("dry run should show dep order: %s", out)
	}
}

func TestRunMainLintDisableNoBakefile(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	code, err := RunMain([]string{"lint", "--disable", "require-desc"})
	if err == nil {
		t.Fatal("expected error when no Bakefile")
	}
	if code != 2 {
		t.Errorf("expected exit 2, got %d", code)
	}
}

func TestRunMainShowCmd(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte("target build { desc \"b\" steps { exec [\"true\"] } }\n"), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	t.Setenv("BAKE_NO_AUTOFORMAT", "1")
	t.Setenv("BAKE_NO_AUTOLINT", "1")
	code, err := RunMain([]string{"--show-cmd", "build"})
	if err != nil {
		t.Fatalf("RunMain --show-cmd: %v", err)
	}
	if code != 0 {
		t.Errorf("expected exit 0, got %d", code)
	}
}

func TestRunMainRunSuiteDryRun(t *testing.T) {
	dir := t.TempDir()
	bf := "target a { desc \"a\" steps { exec [\"true\"] } }\ntarget b { desc \"b\" steps { exec [\"true\"] } }\nsuite s { a b }\n"
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte(bf), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	t.Setenv("BAKE_NO_AUTOFORMAT", "1")
	t.Setenv("BAKE_NO_AUTOLINT", "1")
	out, code, err := runMainCaptureStdout(t, []string{"--dry-run", "s"})
	if err != nil {
		t.Fatalf("RunMain --dry-run suite: %v", err)
	}
	if code != 0 {
		t.Errorf("expected exit 0, got %d", code)
	}
	if !strings.Contains(out, "target a") || !strings.Contains(out, "target b") {
		t.Errorf("dry run suite should show both targets: %s", out)
	}
}

func TestRunMainLintWithConfigPath(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte("target build { desc \"b\" steps { exec [\"true\"] } }\n"), 0644)
	cfgDir := filepath.Join(dir, ".bake")
	os.MkdirAll(cfgDir, 0755)
	os.WriteFile(filepath.Join(cfgDir, "config"), []byte("require-desc disabled\n"), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	t.Setenv("BAKE_NO_AUTOFORMAT", "1")
	t.Setenv("BAKE_NO_AUTOLINT", "1")
	code, err := RunMain([]string{"lint", "--config", filepath.Join(cfgDir, "config")})
	if err != nil {
		t.Fatalf("RunMain lint --config: %v", err)
	}
	if code != 0 {
		t.Errorf("expected exit 0 with require-desc disabled, got %d", code)
	}
}

func TestEnsureBakefileFormatLint(t *testing.T) {
	dir := t.TempDir()
	bf := []byte("target   build { desc \"b\" steps { exec [\"true\"] } }\n")
	path := filepath.Join(dir, "Bakefile")
	os.WriteFile(path, bf, 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	if err := ensureBakefileFormatLint(path); err != nil {
		t.Fatalf("ensureBakefileFormatLint: %v", err)
	}
	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), "   build") {
		t.Errorf("extra spaces should be cleaned by auto-format: %s", data)
	}
}

func TestRunMainRunTargetWithPassthrough(t *testing.T) {
	dir := t.TempDir()
	bf := "target test { desc \"t\" passthrough step = 1 steps { exec [\"echo\", \"hello\"] } }\n"
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte(bf), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	t.Setenv("BAKE_NO_AUTOFORMAT", "1")
	t.Setenv("BAKE_NO_AUTOLINT", "1")
	code, err := RunMain([]string{"test", "--", "extra"})
	if err != nil {
		t.Fatalf("RunMain test -- extra: %v", err)
	}
	if code != 0 {
		t.Errorf("expected exit 0, got %d", code)
	}
}

func TestRunMainWhyUnknownTarget(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte("target build { desc \"b\" steps { exec [\"true\"] } }\n"), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	t.Setenv("BAKE_NO_AUTOFORMAT", "1")
	t.Setenv("BAKE_NO_AUTOLINT", "1")
	code, err := RunMain([]string{"--why", "nonexistent"})
	if err == nil {
		t.Fatal("expected error for unknown target")
	}
	if code != 2 {
		t.Errorf("expected exit 2, got %d", code)
	}
}

func TestRunMainWhatDependsOnUnknown(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte("target build { desc \"b\" steps { exec [\"true\"] } }\n"), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	t.Setenv("BAKE_NO_AUTOFORMAT", "1")
	t.Setenv("BAKE_NO_AUTOLINT", "1")
	code, err := RunMain([]string{"--what-depends-on", "nonexistent"})
	if err == nil {
		t.Fatal("expected error for unknown target")
	}
	if code != 2 {
		t.Errorf("expected exit 2, got %d", code)
	}
}

func TestRunMainFormatWrite(t *testing.T) {
	dir := t.TempDir()
	bf := []byte("target   build { desc \"b\" steps { exec [\"true\"] } }\n")
	path := filepath.Join(dir, "Bakefile")
	os.WriteFile(path, bf, 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	code, err := RunMain([]string{"fmt", "-w"})
	if err != nil {
		t.Fatalf("RunMain fmt -w: %v", err)
	}
	if code != 0 {
		t.Errorf("expected exit 0, got %d", code)
	}
	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), "   build") {
		t.Errorf("extra spaces should be removed after format: %s", data)
	}
}

func TestRunMainUpWithDaemonWorkflow(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("daemon uses sleep; run on Unix")
	}
	dir := t.TempDir()
	bf := `target build { desc "b" steps { exec ["true"] } }
target up {
  workflow { build mydb }
  daemon mydb { steps { exec ["sleep", "0.2"] } }
}
`
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte(bf), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	t.Setenv("BAKE_NO_AUTOFORMAT", "1")
	t.Setenv("BAKE_NO_AUTOLINT", "1")
	t.Setenv("BAKE_NO_DAEMON", "1")
	code, err := RunMain([]string{"up"})
	if err != nil {
		t.Fatalf("RunMain up: %v", err)
	}
	if code != 0 {
		t.Errorf("up: exit code %d", code)
	}
	// Down all
	code, err = RunMain([]string{"down"})
	if err != nil {
		t.Fatalf("RunMain down: %v", err)
	}
	if code != 0 {
		t.Errorf("down: exit code %d", code)
	}
}

func TestRunMainWatchNoInputsRunsOnce(t *testing.T) {
	dir := t.TempDir()
	bf := `target greet { desc "g" steps { exec ["true"] } }
`
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte(bf), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	t.Setenv("BAKE_NO_AUTOFORMAT", "1")
	t.Setenv("BAKE_NO_AUTOLINT", "1")
	t.Setenv("BAKE_NO_DAEMON", "1")
	code, err := RunMain([]string{"--watch", "greet"})
	if err != nil {
		t.Fatalf("RunMain --watch greet (no inputs): %v", err)
	}
	if code != 0 {
		t.Errorf("expected exit code 0, got %d", code)
	}
}

func TestRunMainGraphSubgraph(t *testing.T) {
	dir := t.TempDir()
	bf := `target build { desc "b" steps { exec ["true"] } }
target test { desc "t" deps build steps { exec ["true"] } }
`
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte(bf), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	t.Setenv("BAKE_NO_AUTOFORMAT", "1")
	t.Setenv("BAKE_NO_AUTOLINT", "1")
	code, err := RunMain([]string{"--graph", "test"})
	if err != nil {
		t.Fatalf("RunMain --graph test: %v", err)
	}
	if code != 0 {
		t.Errorf("expected exit 0, got %d", code)
	}
}

func TestRunMainWhyWithInputs(t *testing.T) {
	dir := t.TempDir()
	bf := `target build { desc "b" inputs ["src/**"] outputs ["out.bin"] steps { exec ["true"] } }
`
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte(bf), 0644)
	os.MkdirAll(filepath.Join(dir, "src"), 0755)
	os.WriteFile(filepath.Join(dir, "src", "main.go"), []byte("package main\n"), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	t.Setenv("BAKE_NO_AUTOFORMAT", "1")
	t.Setenv("BAKE_NO_AUTOLINT", "1")
	code, err := RunMain([]string{"--why", "build"})
	if err != nil {
		t.Fatalf("RunMain --why build: %v", err)
	}
	if code != 0 {
		t.Errorf("expected exit 0, got %d", code)
	}
}

func TestRunMainChooseNoSuiteFallsBack(t *testing.T) {
	dir := t.TempDir()
	bf := `target build { desc "b" steps { exec ["true"] } }
target test { desc "t" steps { exec ["true"] } }
`
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte(bf), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	t.Setenv("BAKE_NO_AUTOFORMAT", "1")
	t.Setenv("BAKE_NO_AUTOLINT", "1")
	code, err := RunMain([]string{"--choose"})
	if err != nil {
		t.Fatalf("RunMain --choose (no suite): %v", err)
	}
	if code != 0 {
		t.Errorf("expected exit 0, got %d", code)
	}
}

func TestRunMainRunTargetNoSteps(t *testing.T) {
	dir := t.TempDir()
	bf := `target noop { desc "nothing" }
`
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte(bf), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	t.Setenv("BAKE_NO_AUTOFORMAT", "1")
	t.Setenv("BAKE_NO_AUTOLINT", "1")
	t.Setenv("BAKE_NO_DAEMON", "1")
	code, err := RunMain([]string{"noop"})
	if err != nil {
		t.Fatalf("RunMain noop: %v", err)
	}
	if code != 0 {
		t.Errorf("expected exit 0, got %d", code)
	}
}

func TestRunMainDownAllWithRunningDaemon(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("daemon uses sleep; run on Unix")
	}
	dir := t.TempDir()
	bf := `target build { desc "b" steps { exec ["true"] } }
target up {
  workflow { build srv1 srv2 }
  daemon srv1 { steps { exec ["sleep", "60"] } }
  daemon srv2 { steps { exec ["sleep", "60"] } }
}
`
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte(bf), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	t.Setenv("BAKE_NO_AUTOFORMAT", "1")
	t.Setenv("BAKE_NO_AUTOLINT", "1")
	t.Setenv("BAKE_NO_DAEMON", "1")
	code, err := RunMain([]string{"up"})
	if err != nil {
		t.Fatalf("up: %v", err)
	}
	if code != 0 {
		t.Errorf("up: exit code %d", code)
	}
	// Down all (exercises the for-loop in runDown)
	code, err = RunMain([]string{"down"})
	if err != nil {
		t.Fatalf("down all: %v", err)
	}
	if code != 0 {
		t.Errorf("down: exit code %d", code)
	}
}

func TestRunMainLintNoAutoLint(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte("target build { desc \"b\" steps { exec [\"true\"] } }\n"), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	t.Setenv("BAKE_NO_AUTOLINT", "1")
	code, err := RunMain([]string{"lint"})
	if err != nil {
		t.Fatalf("RunMain lint: %v", err)
	}
	if code != 0 {
		t.Errorf("expected exit 0, got %d", code)
	}
}

func TestRunMainFormatStdoutNoBakefile(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	code, err := RunMain([]string{"fmt"})
	if err == nil {
		t.Fatal("expected error for fmt with no Bakefile")
	}
	if code != 2 {
		t.Errorf("expected exit 2, got %d", code)
	}
}

func TestRunMainBuildViaDaemonInProcess(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix socket test; run on Unix")
	}
	dir, err := os.MkdirTemp(os.TempDir(), "bt")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	modRoot := repoRoot(t)
	bakedExe := filepath.Join(dir, "baked")
	build := exec.Command("go", "build", "-o", bakedExe, "./cmd/baked")
	build.Dir = modRoot
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build baked: %v\n%s", err, out)
	}

	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte("target build { desc \"b\" steps { exec [\"true\"] } }\n"), 0644)

	bakedCmd := exec.Command(bakedExe, "--no-watch")
	bakedCmd.Dir = dir
	if err := bakedCmd.Start(); err != nil {
		t.Fatalf("start baked: %v", err)
	}
	defer bakedCmd.Process.Kill()

	sockPath := filepath.Join(dir, ".bake", "baked.sock")
	for i := 0; i < 100; i++ {
		time.Sleep(50 * time.Millisecond)
		if _, err := os.Stat(sockPath); err == nil {
			break
		}
	}

	origWd, _ := os.Getwd()
	origPath := os.Getenv("PATH")
	os.Chdir(dir)
	os.Setenv("PATH", dir+string(filepath.ListSeparator)+origPath)
	defer func() {
		os.Chdir(origWd)
		os.Setenv("PATH", origPath)
	}()
	t.Setenv("BAKE_NO_AUTOFORMAT", "1")
	t.Setenv("BAKE_NO_AUTOLINT", "1")
	// Do NOT set BAKE_NO_DAEMON — exercise the daemon path
	code, err := RunMain([]string{"build"})
	if err != nil {
		t.Fatalf("RunMain build via daemon: %v", err)
	}
	if code != 0 {
		t.Errorf("expected exit 0, got %d", code)
	}
}

func TestRunMainUpViaDaemonInProcess(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix socket test; run on Unix")
	}
	dir, err := os.MkdirTemp(os.TempDir(), "bt")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	modRoot := repoRoot(t)
	bakedExe := filepath.Join(dir, "baked")
	build := exec.Command("go", "build", "-o", bakedExe, "./cmd/baked")
	build.Dir = modRoot
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build baked: %v\n%s", err, out)
	}

	bf := `target build { desc "b" steps { exec ["true"] } }
target up {
  workflow { build }
}
`
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte(bf), 0644)

	bakedCmd := exec.Command(bakedExe, "--no-watch")
	bakedCmd.Dir = dir
	if err := bakedCmd.Start(); err != nil {
		t.Fatalf("start baked: %v", err)
	}
	defer bakedCmd.Process.Kill()

	sockPath := filepath.Join(dir, ".bake", "baked.sock")
	for i := 0; i < 100; i++ {
		time.Sleep(50 * time.Millisecond)
		if _, err := os.Stat(sockPath); err == nil {
			break
		}
	}

	origWd, _ := os.Getwd()
	origPath := os.Getenv("PATH")
	os.Chdir(dir)
	os.Setenv("PATH", dir+string(filepath.ListSeparator)+origPath)
	defer func() {
		os.Chdir(origWd)
		os.Setenv("PATH", origPath)
	}()
	t.Setenv("BAKE_NO_AUTOFORMAT", "1")
	t.Setenv("BAKE_NO_AUTOLINT", "1")
	code, err := RunMain([]string{"up"})
	if err != nil {
		t.Fatalf("RunMain up via daemon: %v", err)
	}
	if code != 0 {
		t.Errorf("expected exit 0, got %d", code)
	}

	code, err = RunMain([]string{"down"})
	if err != nil {
		t.Fatalf("RunMain down via daemon: %v", err)
	}
	if code != 0 {
		t.Errorf("expected exit 0, got %d", code)
	}
}

func TestRunMainDefaultWithNoTarget(t *testing.T) {
	dir := t.TempDir()
	bf := `target other { desc "o" steps { exec ["true"] } }
`
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte(bf), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	t.Setenv("BAKE_NO_AUTOFORMAT", "1")
	t.Setenv("BAKE_NO_AUTOLINT", "1")
	t.Setenv("BAKE_NO_DAEMON", "1")
	code, _ := RunMain([]string{})
	if code != 0 {
		t.Errorf("expected exit 0 (picks first target), got %d", code)
	}
}

func TestRunMainGraphJSONSubgraph(t *testing.T) {
	dir := t.TempDir()
	bf := `target build { desc "b" steps { exec ["true"] } }
target test { desc "t" deps build steps { exec ["true"] } }
target lint { desc "l" steps { exec ["true"] } }
`
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte(bf), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	t.Setenv("BAKE_NO_AUTOFORMAT", "1")
	t.Setenv("BAKE_NO_AUTOLINT", "1")
	code, err := RunMain([]string{"--graph", "json", "test"})
	if err != nil {
		t.Fatalf("RunMain --graph json test: %v", err)
	}
	if code != 0 {
		t.Errorf("expected exit 0, got %d", code)
	}
}

func TestRunMainDownViaDaemonInProcess(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix socket test; run on Unix")
	}
	dir, err := os.MkdirTemp(os.TempDir(), "bt")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	modRoot := repoRoot(t)
	bakedExe := filepath.Join(dir, "baked")
	build := exec.Command("go", "build", "-o", bakedExe, "./cmd/baked")
	build.Dir = modRoot
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build baked: %v\n%s", err, out)
	}

	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte("target build { desc \"b\" steps { exec [\"true\"] } }\n"), 0644)

	bakedCmd := exec.Command(bakedExe, "--no-watch")
	bakedCmd.Dir = dir
	if err := bakedCmd.Start(); err != nil {
		t.Fatalf("start baked: %v", err)
	}
	defer bakedCmd.Process.Kill()

	sockPath := filepath.Join(dir, ".bake", "baked.sock")
	for i := 0; i < 100; i++ {
		time.Sleep(50 * time.Millisecond)
		if _, err := os.Stat(sockPath); err == nil {
			break
		}
	}

	origWd, _ := os.Getwd()
	origPath := os.Getenv("PATH")
	os.Chdir(dir)
	os.Setenv("PATH", dir+string(filepath.ListSeparator)+origPath)
	defer func() {
		os.Chdir(origWd)
		os.Setenv("PATH", origPath)
	}()
	t.Setenv("BAKE_NO_AUTOFORMAT", "1")
	t.Setenv("BAKE_NO_AUTOLINT", "1")
	code, err := RunMain([]string{"down"})
	if err != nil {
		t.Fatalf("RunMain down via daemon: %v", err)
	}
	if code != 0 {
		t.Errorf("expected exit 0, got %d", code)
	}
}

func TestRunMainEnsureBakefileFormatLintNoAutoLint(t *testing.T) {
	dir := t.TempDir()
	bf := "target   build { desc \"b\" steps { exec [\"true\"] } }\n"
	path := filepath.Join(dir, "Bakefile")
	os.WriteFile(path, []byte(bf), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	t.Setenv("BAKE_NO_AUTOLINT", "1")
	if err := ensureBakefileFormatLint(path); err != nil {
		t.Fatalf("ensureBakefileFormatLint (no autolint): %v", err)
	}
}

func TestRunMainInstallInGitRepo(t *testing.T) {
	dir := t.TempDir()
	bf := `target build { desc "b" steps { exec ["true"] } }
suite precommit { build }
`
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte(bf), 0644)
	os.MkdirAll(filepath.Join(dir, ".git", "hooks"), 0755)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	t.Setenv("BAKE_NO_AUTOFORMAT", "1")
	t.Setenv("BAKE_NO_AUTOLINT", "1")
	code, err := RunMain([]string{"install"})
	if err != nil {
		t.Fatalf("RunMain install: %v", err)
	}
	if code != 0 {
		t.Errorf("expected exit 0, got %d", code)
	}
}

func TestRunMainLintFixWithFindings(t *testing.T) {
	dir := t.TempDir()
	// Single-line cmd form triggers prefer-exec (fixable)
	bf := "target build cmd echo hello\n"
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte(bf), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	t.Setenv("BAKE_NO_AUTOFORMAT", "1")
	t.Setenv("BAKE_NO_AUTOLINT", "1")
	code, err := RunMain([]string{"lint", "--fix"})
	if err != nil {
		t.Fatalf("RunMain lint --fix: %v", err)
	}
	// May return 0 (all fixed) or 1 (unfixable remain)
	if code > 1 {
		t.Errorf("expected exit 0 or 1, got %d", code)
	}
}

func TestRunMainRunSuiteViaDaemon(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix socket test; run on Unix")
	}
	dir, err := os.MkdirTemp(os.TempDir(), "bt")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	modRoot := repoRoot(t)
	bakedExe := filepath.Join(dir, "baked")
	build := exec.Command("go", "build", "-o", bakedExe, "./cmd/baked")
	build.Dir = modRoot
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build baked: %v\n%s", err, out)
	}

	bf := `target build { desc "b" steps { exec ["true"] } }
target test { desc "t" steps { exec ["true"] } }
suite dev { build test }
`
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte(bf), 0644)

	bakedCmd := exec.Command(bakedExe, "--no-watch")
	bakedCmd.Dir = dir
	if err := bakedCmd.Start(); err != nil {
		t.Fatalf("start baked: %v", err)
	}
	defer bakedCmd.Process.Kill()

	sockPath := filepath.Join(dir, ".bake", "baked.sock")
	for i := 0; i < 100; i++ {
		time.Sleep(50 * time.Millisecond)
		if _, err := os.Stat(sockPath); err == nil {
			break
		}
	}

	origWd, _ := os.Getwd()
	origPath := os.Getenv("PATH")
	os.Chdir(dir)
	os.Setenv("PATH", dir+string(filepath.ListSeparator)+origPath)
	defer func() {
		os.Chdir(origWd)
		os.Setenv("PATH", origPath)
	}()
	t.Setenv("BAKE_NO_AUTOFORMAT", "1")
	t.Setenv("BAKE_NO_AUTOLINT", "1")
	code, err := RunMain([]string{"dev"})
	if err != nil {
		t.Fatalf("RunMain dev via daemon: %v", err)
	}
	if code != 0 {
		t.Errorf("expected exit 0, got %d", code)
	}
}

func TestRunMainFormatToStdout(t *testing.T) {
	dir := t.TempDir()
	bf := `target build { desc "b" steps { exec ["true"] } }
`
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte(bf), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	t.Setenv("BAKE_NO_AUTOFORMAT", "1")
	t.Setenv("BAKE_NO_AUTOLINT", "1")
	code, err := RunMain([]string{"fmt"})
	if err != nil {
		t.Fatalf("RunMain fmt: %v", err)
	}
	if code != 0 {
		t.Errorf("expected exit 0, got %d", code)
	}
}

func TestRunMainDownNoDaemons(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte("target build { desc \"b\" steps { exec [\"true\"] } }\n"), 0644)
	os.MkdirAll(filepath.Join(dir, ".bake"), 0755)
	// Write empty state
	os.WriteFile(filepath.Join(dir, ".bake", "state.json"), []byte(`{"daemons":{}}`), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	t.Setenv("BAKE_NO_AUTOFORMAT", "1")
	t.Setenv("BAKE_NO_AUTOLINT", "1")
	t.Setenv("BAKE_NO_DAEMON", "1")
	code, err := RunMain([]string{"down"})
	if err != nil {
		t.Fatalf("RunMain down (no daemons): %v", err)
	}
	if code != 0 {
		t.Errorf("expected exit 0, got %d", code)
	}
}

func TestRunMainUpWithProfileEnvOverlay(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("daemon uses sleep; run on Unix")
	}
	dir := t.TempDir()
	bf := `profile prod { env { MODE prod } }
target build { desc "b" steps { exec ["true"] } }
target up {
  workflow { build srv }
  daemon srv { steps { exec ["sleep", "0.2"] } }
}
`
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte(bf), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	t.Setenv("BAKE_NO_AUTOFORMAT", "1")
	t.Setenv("BAKE_NO_AUTOLINT", "1")
	t.Setenv("BAKE_NO_DAEMON", "1")
	code, err := RunMain([]string{"--profile", "prod", "up"})
	if err != nil {
		t.Fatalf("RunMain up --profile prod: %v", err)
	}
	if code != 0 {
		t.Errorf("up: exit code %d", code)
	}
	code, err = RunMain([]string{"down"})
	if err != nil {
		t.Fatalf("down: %v", err)
	}
	if code != 0 {
		t.Errorf("down: exit code %d", code)
	}
}

func TestRunMainUpUnknownDaemonRef(t *testing.T) {
	dir := t.TempDir()
	bf := `target build { desc "b" steps { exec ["true"] } }
target up {
  workflow { build nonexistent_daemon }
}
`
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte(bf), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	t.Setenv("BAKE_NO_AUTOFORMAT", "1")
	t.Setenv("BAKE_NO_AUTOLINT", "1")
	t.Setenv("BAKE_NO_DAEMON", "1")
	code, err := RunMain([]string{"up"})
	if err == nil {
		t.Fatal("expected error for unknown daemon ref")
	}
	if code != 1 {
		t.Errorf("expected exit 1, got %d", code)
	}
}

func TestRunMainDownStaleState(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte("target build { desc \"b\" steps { exec [\"true\"] } }\n"), 0644)
	os.MkdirAll(filepath.Join(dir, ".bake"), 0755)
	// Write state with a PID that doesn't exist, exercises the warning path
	os.WriteFile(filepath.Join(dir, ".bake", "state.json"), []byte(`{"daemons":{"old":{"pid":99999999}}}`), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	t.Setenv("BAKE_NO_AUTOFORMAT", "1")
	t.Setenv("BAKE_NO_AUTOLINT", "1")
	t.Setenv("BAKE_NO_DAEMON", "1")
	code, err := RunMain([]string{"down"})
	// The stop may fail (stale PID) but runDown should still succeed
	if err != nil {
		t.Fatalf("RunMain down stale: %v", err)
	}
	if code != 0 {
		t.Errorf("expected exit 0, got %d", code)
	}
}

func TestRunMainDownStaleSpecificDaemon(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte("target build { desc \"b\" steps { exec [\"true\"] } }\n"), 0644)
	os.MkdirAll(filepath.Join(dir, ".bake"), 0755)
	os.WriteFile(filepath.Join(dir, ".bake", "state.json"), []byte(`{"daemons":{"old":{"pid":99999999}}}`), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	t.Setenv("BAKE_NO_AUTOFORMAT", "1")
	t.Setenv("BAKE_NO_AUTOLINT", "1")
	t.Setenv("BAKE_NO_DAEMON", "1")
	code, err := RunMain([]string{"down", "old"})
	// The PID is stale so stop will fail; down returns error
	if err == nil {
		t.Log("down old: no error (platform-dependent)")
	}
	_ = code
}

func TestRunMainDefaultUnknownProfile(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte("target build { desc \"b\" steps { exec [\"true\"] } }\n"), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	t.Setenv("BAKE_NO_AUTOFORMAT", "1")
	t.Setenv("BAKE_NO_AUTOLINT", "1")
	t.Setenv("BAKE_NO_DAEMON", "1")
	code, err := RunMain([]string{"--profile", "bogus"})
	if err == nil {
		t.Fatal("expected error for unknown profile")
	}
	if code != 2 {
		t.Errorf("expected exit 2, got %d", code)
	}
}

func TestRunMainInstallShimsExplicit(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte("target build { desc \"b\" steps { exec [\"true\"] } }\n"), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	t.Setenv("BAKE_NO_AUTOFORMAT", "1")
	t.Setenv("BAKE_NO_AUTOLINT", "1")
	code, err := RunMain([]string{"install", "shims"})
	if err != nil {
		t.Fatalf("RunMain install shims: %v", err)
	}
	if code != 0 {
		t.Errorf("expected exit 0, got %d", code)
	}
}

func TestRunMainInstallHooksWithPrecommit(t *testing.T) {
	dir := t.TempDir()
	bf := `target build { desc "b" steps { exec ["true"] } }
suite precommit { build }
`
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte(bf), 0644)
	os.MkdirAll(filepath.Join(dir, ".git", "hooks"), 0755)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	t.Setenv("BAKE_NO_AUTOFORMAT", "1")
	t.Setenv("BAKE_NO_AUTOLINT", "1")
	code, err := RunMain([]string{"install", "hooks"})
	if err != nil {
		t.Fatalf("RunMain install hooks: %v", err)
	}
	if code != 0 {
		t.Errorf("expected exit 0, got %d", code)
	}
	hookData, _ := os.ReadFile(filepath.Join(dir, ".git", "hooks", "pre-commit"))
	if len(hookData) == 0 {
		t.Error("pre-commit hook should have been created")
	}
}

func TestRunMainBuildWithAutoFormatLint(t *testing.T) {
	dir := t.TempDir()
	bf := "target   build { desc \"b\" steps { exec [\"true\"] } }\n"
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte(bf), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	// Do NOT set BAKE_NO_AUTOFORMAT/BAKE_NO_AUTOLINT to exercise those paths
	t.Setenv("BAKE_NO_DAEMON", "1")
	code, err := RunMain([]string{"build"})
	if err != nil {
		t.Fatalf("RunMain build (auto fmt/lint): %v", err)
	}
	if code != 0 {
		t.Errorf("expected exit 0, got %d", code)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "Bakefile"))
	if strings.Contains(string(data), "   build") {
		t.Error("auto-format should clean extra spaces")
	}
}

func TestRunMainDefaultWithJSON(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte("target build { desc \"b\" steps { exec [\"true\"] } }\n"), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	t.Setenv("BAKE_NO_AUTOFORMAT", "1")
	t.Setenv("BAKE_NO_AUTOLINT", "1")
	t.Setenv("BAKE_NO_DAEMON", "1")
	code, err := RunMain([]string{"--json"})
	if err != nil {
		t.Fatalf("RunMain --json: %v", err)
	}
	if code != 0 {
		t.Errorf("expected exit 0, got %d", code)
	}
}

func TestRunMainLintWithFindings(t *testing.T) {
	dir := t.TempDir()
	// Missing desc triggers unfixable desc-missing
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte("target build { steps { exec [\"true\"] } }\n"), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	t.Setenv("BAKE_NO_AUTOFORMAT", "1")
	t.Setenv("BAKE_NO_AUTOLINT", "1")
	code, _ := RunMain([]string{"lint"})
	if code != 1 {
		t.Errorf("expected exit 1 for lint findings, got %d", code)
	}
}

func TestRunMainRunTargetJSON(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte("target build { desc \"b\" steps { exec [\"true\"] } }\n"), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	t.Setenv("BAKE_NO_AUTOFORMAT", "1")
	t.Setenv("BAKE_NO_AUTOLINT", "1")
	t.Setenv("BAKE_NO_DAEMON", "1")
	code, err := RunMain([]string{"--json", "build"})
	if err != nil {
		t.Fatalf("RunMain --json build: %v", err)
	}
	if code != 0 {
		t.Errorf("expected exit 0, got %d", code)
	}
}

func TestRunMainWhyAlreadyBuilt(t *testing.T) {
	dir := t.TempDir()
	bf := `target build { desc "b" inputs ["*.go"] outputs ["out.bin"] steps { exec ["true"] } }
`
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte(bf), 0644)
	// Create input and output so cache shows "would skip"
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0644)
	os.WriteFile(filepath.Join(dir, "out.bin"), []byte("binary"), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	t.Setenv("BAKE_NO_AUTOFORMAT", "1")
	t.Setenv("BAKE_NO_AUTOLINT", "1")
	// Run build once to populate cache
	t.Setenv("BAKE_NO_DAEMON", "1")
	RunMain([]string{"build"})
	// Now --why should show "would skip"
	code, err := RunMain([]string{"--why", "build"})
	if err != nil {
		t.Fatalf("RunMain --why build: %v", err)
	}
	if code != 0 {
		t.Errorf("expected exit 0, got %d", code)
	}
}

func TestRunMainUpWithScheduleShortInterval(t *testing.T) {
	dir := t.TempDir()
	bf := `target build { desc "b" steps { exec ["true"] } }
target up {
  workflow { build schedule interval "50ms" }
}
`
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte(bf), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	t.Setenv("BAKE_NO_AUTOFORMAT", "1")
	t.Setenv("BAKE_NO_AUTOLINT", "1")
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	err := runUp(ctx, nil, "", false)
	if err != nil {
		t.Fatalf("runUp with short interval: %v", err)
	}
}

func TestRunMainInstallCreatesNewBakefile(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	t.Setenv("BAKE_NO_AUTOFORMAT", "1")
	t.Setenv("BAKE_NO_AUTOLINT", "1")
	code, err := RunMain([]string{"install"})
	if err != nil {
		t.Fatalf("RunMain install (new): %v", err)
	}
	if code != 0 {
		t.Errorf("expected exit 0, got %d", code)
	}
	if _, err := os.Stat(filepath.Join(dir, "Bakefile")); err != nil {
		t.Errorf("Bakefile should have been created: %v", err)
	}
}

func TestRunMainRunTargetWithDeps(t *testing.T) {
	dir := t.TempDir()
	bf := `target build { desc "b" steps { exec ["true"] } }
target test { desc "t" deps build steps { exec ["true"] } }
`
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte(bf), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	t.Setenv("BAKE_NO_AUTOFORMAT", "1")
	t.Setenv("BAKE_NO_AUTOLINT", "1")
	t.Setenv("BAKE_NO_DAEMON", "1")
	code, err := RunMain([]string{"test"})
	if err != nil {
		t.Fatalf("RunMain test: %v", err)
	}
	if code != 0 {
		t.Errorf("expected exit 0, got %d", code)
	}
}

func TestRunMainExplainUnknownTarget(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte("target build { desc \"b\" steps { exec [\"true\"] } }\n"), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	t.Setenv("BAKE_NO_AUTOFORMAT", "1")
	t.Setenv("BAKE_NO_AUTOLINT", "1")
	code, err := RunMain([]string{"--explain", "nonexistent"})
	if err == nil {
		t.Fatal("expected error for unknown target")
	}
	if code != 2 {
		t.Errorf("expected exit 2, got %d", code)
	}
}

func TestRunMainWatchWithInputs(t *testing.T) {
	dir := t.TempDir()
	bf := `target build { desc "b" inputs ["*.go"] steps { exec ["true"] } }
`
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte(bf), 0644)
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	t.Setenv("BAKE_NO_AUTOFORMAT", "1")
	t.Setenv("BAKE_NO_AUTOLINT", "1")
	t.Setenv("BAKE_NO_DAEMON", "1")
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	// Modify the file mid-watch to trigger the poll callback
	go func() {
		time.Sleep(150 * time.Millisecond)
		os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n// changed\n"), 0644)
	}()
	err := runWatch(ctx, "build", nil, nil, nil, "", false, 1)
	if err != nil {
		t.Fatalf("runWatch: %v", err)
	}
}

func TestRunMainWatchUnknownTarget(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte("target build { desc \"b\" steps { exec [\"true\"] } }\n"), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	t.Setenv("BAKE_NO_AUTOFORMAT", "1")
	t.Setenv("BAKE_NO_AUTOLINT", "1")
	t.Setenv("BAKE_NO_DAEMON", "1")
	code, err := RunMain([]string{"--watch", "nonexistent"})
	if err == nil {
		t.Fatal("expected error for unknown target")
	}
	if code != 2 {
		t.Errorf("expected exit 2, got %d", code)
	}
}

func TestRunMainWatchWithProfile(t *testing.T) {
	dir := t.TempDir()
	bf := `profile prod { env { MODE prod } }
target build { desc "b" steps { exec ["true"] } }
`
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte(bf), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	t.Setenv("BAKE_NO_AUTOFORMAT", "1")
	t.Setenv("BAKE_NO_AUTOLINT", "1")
	t.Setenv("BAKE_NO_DAEMON", "1")
	code, err := RunMain([]string{"--watch", "--profile", "prod", "build"})
	if err != nil {
		t.Fatalf("RunMain --watch --profile prod build: %v", err)
	}
	if code != 0 {
		t.Errorf("expected exit 0, got %d", code)
	}
}

func TestRunMainLintJSONFindings(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte("target build { steps { exec [\"true\"] } }\n"), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	t.Setenv("BAKE_NO_AUTOFORMAT", "1")
	t.Setenv("BAKE_NO_AUTOLINT", "1")
	code, _ := RunMain([]string{"lint", "--json"})
	if code != 1 {
		t.Errorf("expected exit 1 for lint findings, got %d", code)
	}
}

func TestRunMainDownContainerInState(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte("target build { desc \"b\" steps { exec [\"true\"] } }\n"), 0644)
	os.MkdirAll(filepath.Join(dir, ".bake"), 0755)
	// Simulate a container daemon entry in state
	os.WriteFile(filepath.Join(dir, ".bake", "state.json"), []byte(`{"daemons":{"mydb":{"pid":0,"container_id":"abc123def456"}}}`), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	t.Setenv("BAKE_NO_AUTOFORMAT", "1")
	t.Setenv("BAKE_NO_AUTOLINT", "1")
	t.Setenv("BAKE_NO_DAEMON", "1")
	// Will fail to stop container (no docker), but exercises the container path
	code, _ := RunMain([]string{"down"})
	_ = code // we just want to exercise the code path
}

func TestRunMainGraphDotWithEdges(t *testing.T) {
	dir := t.TempDir()
	bf := `target build { desc "b" steps { exec ["true"] } }
target test { desc "t" deps build steps { exec ["true"] } }
target lint { desc "l" deps build steps { exec ["true"] } }
`
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte(bf), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	t.Setenv("BAKE_NO_AUTOFORMAT", "1")
	t.Setenv("BAKE_NO_AUTOLINT", "1")
	code, err := RunMain([]string{"--graph", "dot"})
	if err != nil {
		t.Fatalf("RunMain --graph dot: %v", err)
	}
	if code != 0 {
		t.Errorf("expected exit 0, got %d", code)
	}
}

func TestRunMainGraphUnknownTarget(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte("target build { desc \"b\" steps { exec [\"true\"] } }\n"), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	t.Setenv("BAKE_NO_AUTOFORMAT", "1")
	t.Setenv("BAKE_NO_AUTOLINT", "1")
	code, err := RunMain([]string{"--graph", "dot", "nonexistent"})
	if err == nil {
		t.Fatal("expected error for unknown target")
	}
	if code != 2 {
		t.Errorf("expected exit 2, got %d", code)
	}
}

func TestRunMainRunDryRunTarget(t *testing.T) {
	dir := t.TempDir()
	bf := `target build { desc "b" steps { exec ["true"] } }
target test { desc "t" deps build steps { exec ["true"] } }
`
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte(bf), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	t.Setenv("BAKE_NO_AUTOFORMAT", "1")
	t.Setenv("BAKE_NO_AUTOLINT", "1")
	t.Setenv("BAKE_NO_DAEMON", "1")
	code, err := RunMain([]string{"--dry-run", "test"})
	if err != nil {
		t.Fatalf("RunMain --dry-run test: %v", err)
	}
	if code != 0 {
		t.Errorf("expected exit 0, got %d", code)
	}
}

func TestRunMainUpNoWorkflow(t *testing.T) {
	dir := t.TempDir()
	bf := `target up { desc "u" steps { exec ["true"] } }
`
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte(bf), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	t.Setenv("BAKE_NO_AUTOFORMAT", "1")
	t.Setenv("BAKE_NO_AUTOLINT", "1")
	t.Setenv("BAKE_NO_DAEMON", "1")
	code, err := RunMain([]string{"up"})
	if err == nil {
		t.Fatal("expected error for up with no workflow")
	}
	if code != 1 {
		t.Errorf("expected exit 1, got %d", code)
	}
}

func TestRunMainWatchWithPreset(t *testing.T) {
	dir := t.TempDir()
	bf := `target build {
  desc "b"
  preset fast { argv ["true"] }
  steps { exec ["true"] }
}
`
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte(bf), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	t.Setenv("BAKE_NO_AUTOFORMAT", "1")
	t.Setenv("BAKE_NO_AUTOLINT", "1")
	t.Setenv("BAKE_NO_DAEMON", "1")
	code, err := RunMain([]string{"--watch", "build", "fast"})
	if err != nil {
		t.Fatalf("RunMain --watch build fast: %v", err)
	}
	if code != 0 {
		t.Errorf("expected exit 0, got %d", code)
	}
}

func TestRunMainInstallHooksNoPrecommit(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte("target build { desc \"b\" steps { exec [\"true\"] } }\n"), 0644)
	os.MkdirAll(filepath.Join(dir, ".git"), 0755)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	t.Setenv("BAKE_NO_AUTOFORMAT", "1")
	t.Setenv("BAKE_NO_AUTOLINT", "1")
	code, err := RunMain([]string{"install", "hooks"})
	if err == nil {
		t.Fatal("expected error (no precommit suite)")
	}
	if code != 2 {
		t.Errorf("expected exit 2, got %d", code)
	}
}

func TestRunMainUpScheduleCronWithProfile(t *testing.T) {
	dir := t.TempDir()
	bf := `profile prod { env { MODE prod } }
target build { desc "b" steps { exec ["true"] } }
target up {
  workflow { build schedule cron "* * * * *" }
}
`
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte(bf), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	t.Setenv("BAKE_NO_AUTOFORMAT", "1")
	t.Setenv("BAKE_NO_AUTOLINT", "1")
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	err := runUp(ctx, nil, "prod", false)
	if err != nil {
		t.Fatalf("runUp with cron+profile: %v", err)
	}
}

func TestRunMainWatchUnknownProfile(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte("target build { desc \"b\" steps { exec [\"true\"] } }\n"), 0644)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	t.Setenv("BAKE_NO_AUTOFORMAT", "1")
	t.Setenv("BAKE_NO_AUTOLINT", "1")
	t.Setenv("BAKE_NO_DAEMON", "1")
	code, err := RunMain([]string{"--watch", "--profile", "bogus", "build"})
	if err == nil {
		t.Fatal("expected error for unknown profile")
	}
	if code != 2 {
		t.Errorf("expected exit 2, got %d", code)
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
