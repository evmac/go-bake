package baked

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/evmac/go-bake/internal/config"
	"github.com/evmac/go-bake/internal/lifecycle"
)

func testCfg(dir string) *config.File {
	return &config.File{
		RootDir: dir,
		Targets: []*config.Target{
			{
				Name:  "build",
				Steps: []config.Step{{Argv: []string{"true"}}},
			},
			{
				Name:  "test",
				Steps: []config.Step{{Argv: []string{"true"}}},
			},
		},
		Suites: []*config.Suite{
			{Name: "dev", Targets: []string{"build", "test"}},
		},
	}
}

func TestRunTargetSuccess(t *testing.T) {
	dir := t.TempDir()
	cfg := testCfg(dir)
	if err := RunTarget(context.Background(), cfg, "build"); err != nil {
		t.Fatalf("RunTarget: %v", err)
	}
}

func TestRunTargetUnknown(t *testing.T) {
	dir := t.TempDir()
	cfg := testCfg(dir)
	err := RunTarget(context.Background(), cfg, "nope")
	if err == nil {
		t.Fatal("expected error for unknown target")
	}
	if err.Error() != `unknown target "nope"` {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRunSuiteSuccess(t *testing.T) {
	dir := t.TempDir()
	cfg := testCfg(dir)
	if err := RunSuite(context.Background(), cfg, "dev"); err != nil {
		t.Fatalf("RunSuite: %v", err)
	}
}

func TestRunSuiteUnknown(t *testing.T) {
	dir := t.TempDir()
	cfg := testCfg(dir)
	err := RunSuite(context.Background(), cfg, "nope")
	if err == nil {
		t.Fatal("expected error for unknown suite")
	}
	if err.Error() != `unknown suite "nope"` {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRunSuiteSkipsUnknownTarget(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.File{
		RootDir: dir,
		Targets: []*config.Target{
			{Name: "build", Steps: []config.Step{{Argv: []string{"true"}}}},
		},
		Suites: []*config.Suite{
			{Name: "broken", Targets: []string{"build", "nonexistent"}},
		},
	}
	if err := RunSuite(context.Background(), cfg, "broken"); err != nil {
		t.Fatalf("RunSuite should skip unknown targets: %v", err)
	}
}

func TestRunUpNoUpTarget(t *testing.T) {
	dir := t.TempDir()
	cfg := testCfg(dir)
	err := RunUp(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected error when no up target")
	}
	if err.Error() != `no target "up" defined` {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRunUpNoWorkflow(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.File{
		RootDir: dir,
		Targets: []*config.Target{
			{Name: "up", Steps: []config.Step{{Argv: []string{"true"}}}},
		},
	}
	err := RunUp(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected error when up has no workflow")
	}
	if err.Error() != `target "up" has no workflow` {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRunUpWithWorkflowTargets(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.File{
		RootDir: dir,
		Targets: []*config.Target{
			{
				Name:     "up",
				Workflow: []string{"build"},
			},
			{
				Name:  "build",
				Steps: []config.Step{{Argv: []string{"true"}}},
			},
		},
	}
	if err := RunUp(context.Background(), cfg); err != nil {
		t.Fatalf("RunUp: %v", err)
	}
}

func TestRunUpUnknownWorkflowRef(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.File{
		RootDir: dir,
		Targets: []*config.Target{
			{
				Name:     "up",
				Workflow: []string{"nonexistent"},
			},
		},
	}
	err := RunUp(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected error for unknown workflow ref")
	}
}

func TestRunUpWithDaemon(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.File{
		RootDir: dir,
		Targets: []*config.Target{
			{
				Name:     "up",
				Workflow: []string{"sleeper"},
				Daemons: []*config.Daemon{
					{
						Name:  "sleeper",
						Steps: []config.Step{{Argv: []string{"sleep", "60"}}},
					},
				},
			},
		},
	}
	if err := RunUp(context.Background(), cfg); err != nil {
		t.Fatalf("RunUp with daemon: %v", err)
	}
	state, err := lifecycle.Load(dir)
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	entry, ok := state.Daemons["sleeper"]
	if !ok {
		t.Fatal("sleeper daemon not in state")
	}
	if entry.PID == 0 {
		t.Error("sleeper PID is 0")
	}
	// Clean up
	_ = lifecycle.StopDaemon(entry.PID)
	_ = lifecycle.RemoveDaemon(dir, "sleeper")
}

func TestRunDownNoState(t *testing.T) {
	dir := t.TempDir()
	if err := RunDown(dir, ""); err != nil {
		t.Fatalf("RunDown with empty state: %v", err)
	}
}

func TestRunDownSpecificDaemonNotInState(t *testing.T) {
	dir := t.TempDir()
	err := RunDown(dir, "redis")
	if err == nil {
		t.Fatal("expected error for daemon not in state")
	}
}

func TestRunDownStopsDaemon(t *testing.T) {
	dir := t.TempDir()
	// Start a sleep process to simulate a daemon
	ctx := context.Background()
	d := &config.Daemon{
		Name:  "testd",
		Steps: []config.Step{{Argv: []string{"sleep", "60"}}},
	}
	pid, _, err := lifecycle.StartDaemon(ctx, dir, d, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("StartDaemon: %v", err)
	}
	if err := lifecycle.AddDaemon(dir, "testd", pid, ""); err != nil {
		t.Fatalf("AddDaemon: %v", err)
	}
	if err := RunDown(dir, "testd"); err != nil {
		t.Fatalf("RunDown: %v", err)
	}
	state, _ := lifecycle.Load(dir)
	if _, ok := state.Daemons["testd"]; ok {
		t.Error("daemon should be removed from state")
	}
}

func TestRunDownAllDaemons(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	for _, name := range []string{"d1", "d2"} {
		d := &config.Daemon{
			Name:  name,
			Steps: []config.Step{{Argv: []string{"sleep", "60"}}},
		}
		pid, _, err := lifecycle.StartDaemon(ctx, dir, d, nil, nil, nil, nil)
		if err != nil {
			t.Fatalf("StartDaemon %s: %v", name, err)
		}
		lifecycle.AddDaemon(dir, name, pid, "")
	}
	if err := RunDown(dir, ""); err != nil {
		t.Fatalf("RunDown all: %v", err)
	}
	state, _ := lifecycle.Load(dir)
	if len(state.Daemons) != 0 {
		t.Errorf("expected 0 daemons, got %d", len(state.Daemons))
	}
}

func TestEnsureFormatLint(t *testing.T) {
	dir := t.TempDir()
	bakePath := filepath.Join(dir, "Bakefile")
	os.WriteFile(bakePath, []byte("target build {\n  steps {\n    exec [\"true\"]\n  }\n}\n"), 0644)
	if err := EnsureFormatLint(bakePath); err != nil {
		t.Fatalf("EnsureFormatLint: %v", err)
	}
	data, _ := os.ReadFile(bakePath)
	if len(data) == 0 {
		t.Fatal("Bakefile empty after format/lint")
	}
}

func TestWatcherSetConfigAndConfig(t *testing.T) {
	w, err := NewWatcher(func() (*config.File, error) {
		return nil, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	if w.Config() != nil {
		t.Error("Config() should be nil initially")
	}
	cfg := &config.File{RootDir: "/tmp/test"}
	w.SetConfig(cfg)
	if got := w.Config(); got != cfg {
		t.Errorf("Config() after SetConfig: got %v, want %v", got, cfg)
	}
}

func TestRunSuiteWithPreset(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.File{
		RootDir: dir,
		Targets: []*config.Target{
			{
				Name:  "test",
				Steps: []config.Step{{Argv: []string{"true"}}},
				Presets: []config.Preset{
					{Name: "cover", Env: map[string]string{"COVER": "1"}},
				},
			},
		},
		Suites: []*config.Suite{
			{Name: "ci", Targets: []string{"test cover"}},
		},
	}
	if err := RunSuite(context.Background(), cfg, "ci"); err != nil {
		t.Fatalf("RunSuite with preset: %v", err)
	}
}

func TestEnsureFormatLintNoAutoformat(t *testing.T) {
	dir := t.TempDir()
	bakePath := filepath.Join(dir, "Bakefile")
	content := "target build {\n  steps {\n    exec [\"true\"]\n  }\n}\n"
	os.WriteFile(bakePath, []byte(content), 0644)
	t.Setenv("BAKE_NO_AUTOFORMAT", "1")
	if err := EnsureFormatLint(bakePath); err != nil {
		t.Fatalf("EnsureFormatLint: %v", err)
	}
	data, _ := os.ReadFile(bakePath)
	if string(data) != content {
		t.Errorf("content changed despite BAKE_NO_AUTOFORMAT")
	}
}

func TestEnsureFormatLintNoAutolint(t *testing.T) {
	dir := t.TempDir()
	bakePath := filepath.Join(dir, "Bakefile")
	os.WriteFile(bakePath, []byte("target build {\n  steps {\n    exec [\"true\"]\n  }\n}\n"), 0644)
	t.Setenv("BAKE_NO_AUTOLINT", "1")
	if err := EnsureFormatLint(bakePath); err != nil {
		t.Fatalf("EnsureFormatLint: %v", err)
	}
}

func TestEnsureFormatLintBothDisabled(t *testing.T) {
	dir := t.TempDir()
	bakePath := filepath.Join(dir, "Bakefile")
	content := "target build {\n  steps {\n    exec [\"true\"]\n  }\n}\n"
	os.WriteFile(bakePath, []byte(content), 0644)
	t.Setenv("BAKE_NO_AUTOFORMAT", "1")
	t.Setenv("BAKE_NO_AUTOLINT", "1")
	if err := EnsureFormatLint(bakePath); err != nil {
		t.Fatalf("EnsureFormatLint: %v", err)
	}
	data, _ := os.ReadFile(bakePath)
	if string(data) != content {
		t.Errorf("content should be unchanged")
	}
}

func TestFormatAtPathReadOnly(t *testing.T) {
	dir := t.TempDir()
	bakePath := filepath.Join(dir, "Bakefile")
	content := "target build {\n  steps {\n    exec [\"true\"]\n  }\n}\n"
	os.WriteFile(bakePath, []byte(content), 0644)
	if err := formatAtPath(bakePath, false); err != nil {
		t.Fatalf("formatAtPath(write=false): %v", err)
	}
	data, _ := os.ReadFile(bakePath)
	if string(data) != content {
		t.Errorf("write=false should not modify file")
	}
}

func TestFormatAtPathBadFile(t *testing.T) {
	err := formatAtPath("/nonexistent/Bakefile", true)
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestLintAtPathBadFile(t *testing.T) {
	err := lintAtPath("/nonexistent/Bakefile", true)
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestLintAtPathWithFixableFindings(t *testing.T) {
	dir := t.TempDir()
	bakePath := filepath.Join(dir, "Bakefile")
	// Single-line cmd form triggers prefer-exec (fixable)
	os.WriteFile(bakePath, []byte("target build cmd echo hello\n"), 0644)
	if err := lintAtPath(bakePath, true); err != nil {
		t.Fatalf("lintAtPath with fix: %v", err)
	}
	data, _ := os.ReadFile(bakePath)
	if len(data) == 0 {
		t.Fatal("Bakefile empty after lint fix")
	}
}

func TestLintAtPathNoFix(t *testing.T) {
	dir := t.TempDir()
	bakePath := filepath.Join(dir, "Bakefile")
	os.WriteFile(bakePath, []byte("target build {\n  steps {\n    exec [\"true\"]\n  }\n}\n"), 0644)
	if err := lintAtPath(bakePath, false); err != nil {
		t.Fatalf("lintAtPath no fix: %v", err)
	}
}

func TestRunUpWithContainerDaemon(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.File{
		RootDir: dir,
		Targets: []*config.Target{
			{
				Name:     "up",
				Workflow: []string{"build", "mydb"},
				Daemons: []*config.Daemon{
					{
						Name:  "mydb",
						Steps: []config.Step{{Argv: []string{"sleep", "60"}}},
					},
				},
			},
			{Name: "build", Steps: []config.Step{{Argv: []string{"true"}}}},
		},
	}
	if err := RunUp(context.Background(), cfg); err != nil {
		t.Fatalf("RunUp with workflow+daemon: %v", err)
	}
	state, _ := lifecycle.Load(dir)
	if _, ok := state.Daemons["mydb"]; !ok {
		t.Error("mydb daemon should be in state")
	}
	_ = RunDown(dir, "")
}

func TestLoadFromSubdir(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte("target build { desc \"b\" steps { exec [\"true\"] } }\n"), 0644)
	sub := filepath.Join(dir, "sub")
	os.MkdirAll(sub, 0755)
	rootDir, cfg, err := Load(sub)
	if err != nil {
		t.Fatalf("Load from subdir: %v", err)
	}
	if rootDir != dir {
		t.Errorf("rootDir=%q, want %q", rootDir, dir)
	}
	if cfg == nil || len(cfg.Targets) != 1 {
		t.Errorf("unexpected config: %+v", cfg)
	}
}
