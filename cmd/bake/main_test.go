package main

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

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
	if err := runDefault(nil, nil, "", false); err != nil {
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
	if err := runTarget(cfg, "build", tgt, nil, nil, nil, nil, nil, false); err != nil {
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
	err = runTarget(cfg, "ghost", nil, nil, nil, nil, nil, nil, false)
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
	if err := runTarget(cfg, "build", tgt, declared, map[string]string{"live": "k"}, nil, nil, nil, false); err != nil {
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
	runDryRun(cfg, "build", cfg.TargetByName("build"), nil, nil, nil, nil)
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
	if err := runTarget(cfg, "noop", tgt, nil, nil, nil, nil, nil, false); err != nil {
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
