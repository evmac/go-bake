package runner

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/evmac/go-bake/internal/cache"
	"github.com/evmac/go-bake/internal/config"
)

// systemTouch returns a path to touch that is not shadowed by .bake/bin in CI.
func systemTouch() string {
	if runtime.GOOS == "windows" {
		return "touch"
	}
	return "/usr/bin/touch"
}

// stepTestF runs "test -f <path>" via sh so the shell builtin is used (avoids
// .bake/bin/test shim in CI and works on macOS where /usr/bin/test does not exist).
func stepTestF(path string) config.Step {
	return config.Step{Runner: "sh", Argv: []string{"test", "-f", path}}
}

func TestRunEcho(t *testing.T) {
	cfg := &config.File{
		RootDir: t.TempDir(),
		Targets: []*config.Target{
			{
				Name: "echo",
				Steps: []config.Step{
					{Argv: []string{"echo", "hello"}},
				},
			},
		},
	}
	err := Run(context.Background(), cfg, "echo", RunOptions{RootDir: cfg.RootDir})
	if err != nil {
		t.Fatal(err)
	}
}

// TestRunWithEventWriter exercises emitEvent (JSON event stream).
func TestRunWithEventWriter(t *testing.T) {
	var buf bytes.Buffer
	cfg := &config.File{
		RootDir: t.TempDir(),
		Targets: []*config.Target{
			{Name: "ev", Steps: []config.Step{{Argv: []string{"echo", "ok"}}}},
		},
	}
	err := Run(context.Background(), cfg, "ev", RunOptions{RootDir: cfg.RootDir, EventWriter: &buf})
	if err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, `"event":"run_start"`) || !strings.Contains(out, `"event":"run_end"`) {
		t.Errorf("expected run_start and run_end events, got %s", out)
	}
}

// TestRunUnknownTargetWithEventWriter exercises emitEvent on error path (errMsg).
func TestRunUnknownTargetWithEventWriter(t *testing.T) {
	var buf bytes.Buffer
	cfg := &config.File{RootDir: t.TempDir(), Targets: []*config.Target{{Name: "a", Steps: []config.Step{{Argv: []string{"true"}}}}}}
	err := Run(context.Background(), cfg, "nonexistent", RunOptions{RootDir: cfg.RootDir, EventWriter: &buf})
	if err == nil {
		t.Fatal("expected error for unknown target")
	}
	out := buf.String()
	if !strings.Contains(out, `"event":"run_end"`) || !strings.Contains(out, "unknown target") {
		t.Errorf("expected run_end with message, got %s", out)
	}
}

// TestRunWithPool exercises getPoolSem (target with pool).
func TestRunWithPool(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.File{
		RootDir: dir,
		Targets: []*config.Target{
			{Name: "p", Pool: "my-pool", Steps: []config.Step{{Argv: []string{"echo", "ok"}}}},
		},
	}
	var poolSems sync.Map
	err := Run(context.Background(), cfg, "p", RunOptions{RootDir: dir, PoolSems: &poolSems})
	if err != nil {
		t.Fatal(err)
	}
}

// TestRunWithMutex exercises getMutex (target with mutex).
func TestRunWithMutex(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.File{
		RootDir: dir,
		Targets: []*config.Target{
			{Name: "m", Mutex: "my-mutex", Steps: []config.Step{{Argv: []string{"echo", "ok"}}}},
		},
	}
	var mutexes sync.Map
	err := Run(context.Background(), cfg, "m", RunOptions{RootDir: dir, Mutexes: &mutexes})
	if err != nil {
		t.Fatal(err)
	}
}

// TestRunTargetWithImageRequiresDocker: target with image runs in container when Docker available;
// when Docker unavailable or image missing, returns an error mentioning Docker/image.
func TestRunTargetWithImageRequiresDocker(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.File{
		RootDir: dir,
		Targets: []*config.Target{
			{Name: "in-container", Image: "alpine:3.19", Steps: []config.Step{{Argv: []string{"echo", "ok"}}}},
		},
	}
	err := Run(context.Background(), cfg, "in-container", RunOptions{RootDir: dir})
	if err == nil {
		return // Docker available and ran successfully
	}
	msg := err.Error()
	if !strings.Contains(msg, "Docker") && !strings.Contains(msg, "image") && !strings.Contains(msg, "container") && !strings.Contains(msg, "pull") && !strings.Contains(msg, "daemon") {
		t.Errorf("error for target with image should mention Docker/image/container/pull/daemon: %q", msg)
	}
}

// TestRunTargetWithImageUnsafeRunsOnHost: target with image and unsafe runs on host (no container).
func TestRunTargetWithImageUnsafeRunsOnHost(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.File{
		RootDir: dir,
		Targets: []*config.Target{
			{Name: "unsafe-host", Image: "alpine:3.19", Unsafe: true, Steps: []config.Step{{Argv: []string{"echo", "on-host"}}}},
		},
	}
	err := Run(context.Background(), cfg, "unsafe-host", RunOptions{RootDir: dir})
	if err != nil {
		t.Fatal(err)
	}
}

// TestRunMaxParallel exercises LevelOrder and parallel execution (emitEvent on parallel path).
func TestRunMaxParallel(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.File{
		RootDir: dir,
		Targets: []*config.Target{
			{Name: "a", Deps: []string{"b", "c"}, Steps: []config.Step{{Argv: []string{"echo", "a"}}}},
			{Name: "b", Steps: []config.Step{{Argv: []string{"echo", "b"}}}},
			{Name: "c", Steps: []config.Step{{Argv: []string{"echo", "c"}}}},
		},
	}
	var buf bytes.Buffer
	err := Run(context.Background(), cfg, "a", RunOptions{RootDir: dir, MaxParallel: 2, EventWriter: &buf})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), `"event":"run_end"`) {
		t.Error("expected run_end event")
	}
}

// TestRunSequentialDepFailure covers Run when a dependency fails (emitEvent run_end, errMsg).
func TestRunSequentialDepFailure(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.File{
		RootDir: dir,
		Targets: []*config.Target{
			{Name: "bad", Steps: []config.Step{{Argv: []string{"sh", "-c", "exit 1"}}}},
			{Name: "top", Deps: []string{"bad"}, Steps: []config.Step{{Argv: []string{"echo", "ok"}}}},
		},
	}
	var buf bytes.Buffer
	err := Run(context.Background(), cfg, "top", RunOptions{RootDir: dir, EventWriter: &buf})
	if err == nil {
		t.Fatal("expected error when dep fails")
	}
	if !strings.Contains(err.Error(), "bad") {
		t.Errorf("error should mention dep name: %v", err)
	}
	if !strings.Contains(buf.String(), `"event":"run_end"`) || !strings.Contains(buf.String(), "exit status") {
		t.Errorf("expected run_end with error message: %s", buf.String())
	}
}

// TestRunParallelOneFails covers parallel path when one target in a level fails (firstErr, run_end false).
func TestRunParallelOneFails(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.File{
		RootDir: dir,
		Targets: []*config.Target{
			{Name: "a", Deps: []string{"b", "c"}, Steps: []config.Step{{Argv: []string{"echo", "a"}}}},
			{Name: "b", Steps: []config.Step{{Argv: []string{"sh", "-c", "exit 1"}}}},
			{Name: "c", Steps: []config.Step{{Argv: []string{"echo", "c"}}}},
		},
	}
	var buf bytes.Buffer
	err := Run(context.Background(), cfg, "a", RunOptions{RootDir: dir, MaxParallel: 2, EventWriter: &buf})
	if err == nil {
		t.Fatal("expected error when one dep fails")
	}
	if !strings.Contains(buf.String(), `"event":"run_end"`) {
		t.Errorf("expected run_end event: %s", buf.String())
	}
}

// TestRunTopoOrderError covers Run when TopoOrder fails (unknown dep in DAG).
func TestRunTopoOrderError(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.File{
		RootDir: dir,
		Targets: []*config.Target{
			{Name: "a", Deps: []string{"nonexistent"}, Steps: []config.Step{{Argv: []string{"echo", "a"}}}},
		},
	}
	var buf bytes.Buffer
	err := Run(context.Background(), cfg, "a", RunOptions{RootDir: dir, EventWriter: &buf})
	if err == nil {
		t.Fatal("expected error for unknown dep")
	}
	if !strings.Contains(buf.String(), `"event":"run_end"`) {
		t.Errorf("expected run_end: %s", buf.String())
	}
}

// TestRunWithTimingReporter covers TimingReporter callback.
func TestRunWithTimingReporter(t *testing.T) {
	dir := t.TempDir()
	var reported []struct {
		name string
		d    time.Duration
	}
	cfg := &config.File{
		RootDir: dir,
		Targets: []*config.Target{
			{Name: "t", Steps: []config.Step{{Argv: []string{"echo", "ok"}}}},
		},
	}
	opts := RunOptions{
		RootDir: dir,
		TimingReporter: func(name string, d time.Duration) {
			reported = append(reported, struct {
				name string
				d    time.Duration
			}{name, d})
		},
	}
	err := Run(context.Background(), cfg, "t", opts)
	if err != nil {
		t.Fatal(err)
	}
	if len(reported) != 1 || reported[0].name != "t" || reported[0].d < 0 {
		t.Errorf("TimingReporter: got %v", reported)
	}
}

// TestRunWithArtifactReporter covers ArtifactReporter callback after successful run with outputs.
func TestRunWithArtifactReporter(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "in"), []byte("x"), 0644)
	var reported []struct {
		name  string
		paths []string
	}
	cfg := &config.File{
		RootDir: dir,
		Targets: []*config.Target{
			{
				Name:    "build",
				Inputs:  []string{"in"},
				Outputs: []string{"out"},
				Steps:   []config.Step{{Argv: []string{"sh", "-c", "cp in out"}}},
			},
		},
	}
	opts := RunOptions{
		RootDir: dir,
		ArtifactReporter: func(name string, paths []string) {
			reported = append(reported, struct {
				name  string
				paths []string
			}{name, paths})
		},
	}
	err := Run(context.Background(), cfg, "build", opts)
	if err != nil {
		t.Fatal(err)
	}
	if len(reported) != 1 || reported[0].name != "build" || len(reported[0].paths) == 0 {
		t.Errorf("ArtifactReporter: got %v", reported)
	}
	if !strings.Contains(reported[0].paths[0], "out") {
		t.Errorf("expected output path in report: %v", reported[0].paths)
	}
}

// TestRunCacheSkipWithEventWriter covers cache_skip event when second run is up to date.
func TestRunCacheSkipWithEventWriter(t *testing.T) {
	dir := t.TempDir()
	inPath := filepath.Join(dir, "in")
	os.WriteFile(inPath, []byte("x"), 0644)
	cfg := &config.File{
		RootDir: dir,
		Targets: []*config.Target{
			{
				Name:    "cached",
				Inputs:  []string{"in"},
				Outputs: []string{"out"},
				Steps:   []config.Step{{Argv: []string{"sh", "-c", "cp in out"}}},
			},
		},
	}
	opts := RunOptions{RootDir: dir}
	err := Run(context.Background(), cfg, "cached", opts)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	opts.EventWriter = &buf
	err = Run(context.Background(), cfg, "cached", opts)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), `"event":"cache_skip"`) {
		t.Errorf("expected cache_skip event on second run: %s", buf.String())
	}
}

// TestRunWithShowCmd exercises printCmdToStderr (ShowCmd: true).
func TestRunWithShowCmd(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.File{
		RootDir: dir,
		Targets: []*config.Target{
			{Name: "show", Steps: []config.Step{{Argv: []string{"echo", "ok"}}}},
		},
	}
	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w
	err = Run(context.Background(), cfg, "show", RunOptions{RootDir: dir, ShowCmd: true})
	os.Stderr = oldStderr
	w.Close()
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	for {
		var b [256]byte
		n, _ := r.Read(b[:])
		if n == 0 {
			break
		}
		buf.Write(b[:n])
	}
	if !bytes.Contains(buf.Bytes(), []byte("+")) || !bytes.Contains(buf.Bytes(), []byte("echo")) {
		t.Errorf("ShowCmd should print command to stderr; got %q", buf.String())
	}
}

// TestRunStripBakeBinFromPath ensures .bake/bin is removed from PATH so steps see real binaries.
func TestRunStripBakeBinFromPath(t *testing.T) {
	dir := t.TempDir()
	bakeBin := filepath.Join(dir, ".bake", "bin")
	if err := os.MkdirAll(bakeBin, 0755); err != nil {
		t.Fatal(err)
	}
	pathVal := bakeBin + string(filepath.ListSeparator) + os.Getenv("PATH")
	cfg := &config.File{
		RootDir: dir,
		Targets: []*config.Target{
			{
				Name:  "check",
				Steps: []config.Step{{Runner: "sh", Argv: []string{"test", "0", "-eq", "0"}}},
			},
		},
	}
	opts := RunOptions{RootDir: dir, TargetEnv: map[string]string{"PATH": pathVal}}
	if err := Run(context.Background(), cfg, "check", opts); err != nil {
		t.Fatal(err)
	}
}

func TestRunWithDeps(t *testing.T) {
	dir := t.TempDir()
	firstDone := filepath.Join(dir, "first.done")
	cfg := &config.File{
		RootDir: dir,
		Targets: []*config.Target{
			{Name: "first", Steps: []config.Step{{Argv: []string{systemTouch(), firstDone}}}},
			{Name: "second", Deps: []string{"first"}, Steps: []config.Step{stepTestF(firstDone)}},
		},
	}
	err := Run(context.Background(), cfg, "second", RunOptions{RootDir: dir})
	if err != nil {
		t.Fatal(err)
	}
}

func TestRunWithPreset(t *testing.T) {
	dir := t.TempDir()
	outFile := filepath.Join(dir, "preset_out.txt")
	cfg := &config.File{
		RootDir: dir,
		Targets: []*config.Target{
			{
				Name: "test",
				Steps: []config.Step{
					{Argv: []string{"sh", "-c", "echo \"$0\" > " + outFile}},
				},
				PassthroughStep: 1,
				Presets: []config.Preset{
					{Name: "cover", Argv: []string{"-coverprofile=coverage.out"}},
					{Name: "extra", Env: map[string]string{"BAKE_VAR": "1"}},
				},
			},
		},
	}
	// Run with preset "cover": argv should be appended to step 1
	err := Run(context.Background(), cfg, "test", RunOptions{
		RootDir: dir,
		Preset:  &cfg.Targets[0].Presets[0],
	})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(data)); got != "-coverprofile=coverage.out" {
		t.Errorf("preset argv: got %q", got)
	}
	// Run with preset "extra" (env only): step should see BAKE_VAR=1
	envOut := filepath.Join(dir, "env_out.txt")
	cfg.Targets[0].Steps[0] = config.Step{Argv: []string{"sh", "-c", "echo \"$BAKE_VAR\" > " + envOut}}
	err = Run(context.Background(), cfg, "test", RunOptions{
		RootDir: dir,
		Preset:  &cfg.Targets[0].Presets[1],
	})
	if err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(envOut)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(data)); got != "1" {
		t.Errorf("preset env: got %q", got)
	}
}

func TestRunPassthrough(t *testing.T) {
	cfg := &config.File{
		RootDir: t.TempDir(),
		Targets: []*config.Target{
			{
				Name: "pt",
				Steps: []config.Step{
					{Argv: []string{"sh", "-c", "echo $1"}},
				},
				PassthroughStep: 1,
			},
		},
	}
	// Passthrough args go to step 1 (first step)
	err := Run(context.Background(), cfg, "pt", RunOptions{
		RootDir:           cfg.RootDir,
		PassthroughByStep: map[int][]string{1: {"hi"}},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestRunWithEnv(t *testing.T) {
	cfg := &config.File{
		RootDir: t.TempDir(),
		Targets: []*config.Target{
			{
				Name: "env",
				Env:  map[string]string{"BAKE_TEST_RUN": "1"},
				Steps: []config.Step{
					{Argv: []string{"sh", "-c", "test \"$BAKE_TEST_RUN\" = 1"}},
				},
			},
		},
	}
	err := Run(context.Background(), cfg, "env", RunOptions{RootDir: cfg.RootDir})
	if err != nil {
		t.Fatal(err)
	}
}

func TestRunWithShellStep(t *testing.T) {
	// Shell step hits joinArgv, quoteForShell, escapeSingleQuoted (argv with space/special chars).
	cfg := &config.File{
		RootDir: t.TempDir(),
		Targets: []*config.Target{
			{
				Name: "shell",
				Steps: []config.Step{{
					Runner: "sh",
					Argv:   []string{"echo", "hello world", "foo'bar"},
				}},
			},
		},
	}
	err := Run(context.Background(), cfg, "shell", RunOptions{RootDir: cfg.RootDir})
	if err != nil {
		t.Fatal(err)
	}
}

func TestRunWithStepEnvAndTemplate(t *testing.T) {
	// Target with Args + Env expansion and step-level Env hits stepSignature and runTargetSteps expansion paths.
	dir := t.TempDir()
	cfg := &config.File{
		RootDir: dir,
		Targets: []*config.Target{
			{
				Name: "t",
				Args: []config.ArgDecl{{Name: "x", Type: "string", Default: "default"}},
				Env:  map[string]string{"OUT": "{{.x}}"},
				Steps: []config.Step{
					{Argv: []string{"sh", "-c", "test \"$OUT\" = \"overridden\""}, Env: map[string]string{"OUT": "{{.x}}"}},
				},
			},
		},
	}
	opts := RunOptions{RootDir: dir, DeclaredArgs: map[string]string{"x": "overridden"}}
	err := Run(context.Background(), cfg, "t", opts)
	if err != nil {
		t.Fatal(err)
	}
}

func TestRunWithInputsOutputsAndTemplate(t *testing.T) {
	// Target with inputs/outputs and DeclaredArgs exercises stepSignature expansion path in cache key.
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "in"), []byte("x"), 0644)
	cfg := &config.File{
		RootDir: dir,
		Targets: []*config.Target{
			{
				Name:    "t",
				Inputs:  []string{"in"},
				Outputs: []string{"out"},
				Args:    []config.ArgDecl{{Name: "v", Type: "string", Default: "1"}},
				Env:     map[string]string{"V": "{{.v}}"},
				Steps:   []config.Step{{Argv: []string{"sh", "-c", "cp in out"}}},
			},
		},
	}
	opts := RunOptions{RootDir: dir, DeclaredArgs: map[string]string{"v": "2"}}
	err := Run(context.Background(), cfg, "t", opts)
	if err != nil {
		t.Fatal(err)
	}
}

// TestRunWithInputsOutputsAndStepEnv exercises stepSignature with step-level Env expansion.
func TestRunWithInputsOutputsAndStepEnv(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "in"), []byte("x"), 0644)
	cfg := &config.File{
		RootDir: dir,
		Targets: []*config.Target{
			{
				Name:    "t",
				Inputs:  []string{"in"},
				Outputs: []string{"out"},
				Args:    []config.ArgDecl{{Name: "x", Type: "string", Default: "d"}},
				Steps: []config.Step{{
					Argv: []string{"sh", "-c", "cp in out"},
					Env:  map[string]string{"STEP_VAR": "{{.x}}"},
				}},
			},
		},
	}
	opts := RunOptions{RootDir: dir, DeclaredArgs: map[string]string{"x": "val"}}
	err := Run(context.Background(), cfg, "t", opts)
	if err != nil {
		t.Fatal(err)
	}
}

func TestRunWhenEnv(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "ran")
	cfg := &config.File{
		RootDir: dir,
		Targets: []*config.Target{
			{
				Name:    "guarded",
				WhenEnv: "BAKE_TEST_WHEN_SET",
				Steps:   []config.Step{{Argv: []string{systemTouch(), marker}}},
			},
		},
	}
	// Unset: target should be skipped (success, no steps run).
	err := Run(context.Background(), cfg, "guarded", RunOptions{RootDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Error("when env unset: target should have been skipped, but marker was created")
	}
	// Set via process env: target should run.
	t.Setenv("BAKE_TEST_WHEN_SET", "1")
	err = Run(context.Background(), cfg, "guarded", RunOptions{RootDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Errorf("when env set: expected marker file, got %v", err)
	}
}

func TestRunWithCacheHitAndMiss(t *testing.T) {
	dir := t.TempDir()
	inPath := filepath.Join(dir, "in")
	outPath := filepath.Join(dir, "out")
	markerPath := filepath.Join(dir, "marker")
	if err := os.WriteFile(inPath, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	cfg := &config.File{
		RootDir: dir,
		Targets: []*config.Target{
			{
				Name:    "build",
				Inputs:  []string{"in"},
				Outputs: []string{"out"},
				Steps:   []config.Step{{Argv: []string{"sh", "-c", "cp in out && echo run >> marker"}}},
			},
		},
	}
	opts := RunOptions{RootDir: dir}
	countMarkerLines := func() int {
		b, _ := os.ReadFile(markerPath)
		return len(bytes.Split(bytes.TrimSpace(b), []byte("\n")))
	}

	// First run: no cache, steps run.
	err := Run(context.Background(), cfg, "build", opts)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(outPath); err != nil {
		t.Fatal("expected out to exist after first run:", err)
	}
	if countMarkerLines() != 1 {
		t.Fatalf("after first run: expected 1 marker line, got %d", countMarkerLines())
	}

	// Second run: cache hit, steps should not run.
	err = Run(context.Background(), cfg, "build", opts)
	if err != nil {
		t.Fatal(err)
	}
	if countMarkerLines() != 1 {
		t.Errorf("cache hit: expected marker still 1 line, got %d (step ran when it should skip)", countMarkerLines())
	}

	// Change input; third run: cache miss, steps run again.
	if err := os.WriteFile(inPath, []byte("changed"), 0644); err != nil {
		t.Fatal(err)
	}
	err = Run(context.Background(), cfg, "build", opts)
	if err != nil {
		t.Fatal(err)
	}
	if countMarkerLines() != 2 {
		t.Errorf("cache miss: expected 2 marker lines, got %d", countMarkerLines())
	}
}

func TestRunWithPresetSteps(t *testing.T) {
	dir := t.TempDir()
	outFile := filepath.Join(dir, "preset_steps_out.txt")
	cfg := &config.File{
		RootDir: dir,
		Targets: []*config.Target{
			{
				Name:  "test",
				Steps: []config.Step{{Argv: []string{"echo", "original"}}},
				Presets: []config.Preset{
					{
						Name: "bench",
						Steps: []config.Step{
							{Argv: []string{"sh", "-c", "echo bench > " + outFile}},
						},
					},
				},
			},
		},
	}
	err := Run(context.Background(), cfg, "test", RunOptions{
		RootDir: dir,
		Preset:  &cfg.Targets[0].Presets[0],
	})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("preset steps should have run: %v", err)
	}
	if got := strings.TrimSpace(string(data)); got != "bench" {
		t.Errorf("preset steps: got %q, want %q", got, "bench")
	}
}

func TestRunWithPresetEnvOverlay(t *testing.T) {
	dir := t.TempDir()
	outFile := filepath.Join(dir, "preset_env_out.txt")
	cfg := &config.File{
		RootDir: dir,
		Targets: []*config.Target{
			{
				Name: "test",
				Env:  map[string]string{"MODE": "normal"},
				Steps: []config.Step{
					{Argv: []string{"sh", "-c", "echo $MODE > " + outFile}},
				},
				Presets: []config.Preset{
					{Name: "cover", Env: map[string]string{"MODE": "coverage"}},
				},
			},
		},
	}
	err := Run(context.Background(), cfg, "test", RunOptions{
		RootDir: dir,
		Preset:  &cfg.Targets[0].Presets[0],
	})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(data)); got != "coverage" {
		t.Errorf("preset env overlay: got %q, want %q", got, "coverage")
	}
}

func TestRunWithPresetEnvNoTargetEnv(t *testing.T) {
	dir := t.TempDir()
	outFile := filepath.Join(dir, "preset_env2.txt")
	cfg := &config.File{
		RootDir: dir,
		Targets: []*config.Target{
			{
				Name: "test",
				Steps: []config.Step{
					{Argv: []string{"sh", "-c", "echo $PVAR > " + outFile}},
				},
				Presets: []config.Preset{
					{Name: "p", Env: map[string]string{"PVAR": "frompreset"}},
				},
			},
		},
	}
	err := Run(context.Background(), cfg, "test", RunOptions{
		RootDir: dir,
		Preset:  &cfg.Targets[0].Presets[0],
	})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(data)); got != "frompreset" {
		t.Errorf("preset env (no target env): got %q, want %q", got, "frompreset")
	}
}

func TestRunWithShowCmdShellRunner(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.File{
		RootDir: dir,
		Targets: []*config.Target{
			{Name: "sh", Steps: []config.Step{{Runner: "sh", Argv: []string{"echo", "hello"}}}},
		},
	}
	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w
	err = Run(context.Background(), cfg, "sh", RunOptions{RootDir: dir, ShowCmd: true})
	os.Stderr = oldStderr
	w.Close()
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	for {
		var b [256]byte
		n, _ := r.Read(b[:])
		if n == 0 {
			break
		}
		buf.Write(b[:n])
	}
	if !bytes.Contains(buf.Bytes(), []byte("+ sh -c")) {
		t.Errorf("ShowCmd with shell runner should print '+ sh -c ...'; got %q", buf.String())
	}
}

func TestRunWithPresetStepsAndCache(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "in"), []byte("x"), 0644)
	cfg := &config.File{
		RootDir: dir,
		Targets: []*config.Target{
			{
				Name:    "t",
				Inputs:  []string{"in"},
				Outputs: []string{"out"},
				Steps:   []config.Step{{Argv: []string{"sh", "-c", "cp in out"}}},
				Presets: []config.Preset{
					{
						Name:  "alt",
						Steps: []config.Step{{Argv: []string{"sh", "-c", "cp in out && echo alt >> out"}}},
					},
				},
			},
		},
	}
	err := Run(context.Background(), cfg, "t", RunOptions{
		RootDir: dir,
		Preset:  &cfg.Targets[0].Presets[0],
	})
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "out"))
	if !strings.Contains(string(data), "alt") {
		t.Errorf("preset steps should have been used: %s", data)
	}
}

func TestWhyReasonsInputHashMismatchNoChanges(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "in"), []byte("x"), 0644)
	tgt := &config.Target{
		Name:    "t",
		Inputs:  []string{"in"},
		Outputs: []string{"out"},
		Steps:   []config.Step{{Argv: []string{"sh", "-c", "cp in out"}}},
	}
	cfg := &config.File{RootDir: dir, Targets: []*config.Target{tgt}}
	opts := RunOptions{RootDir: dir}
	Run(context.Background(), cfg, "t", opts)

	tgt.Steps = []config.Step{{Argv: []string{"sh", "-c", "cp in out && echo v2"}}}
	reasons, err := WhyReasons(dir, tgt, opts, nil)
	if err != nil {
		t.Fatal(err)
	}
	hasRun := false
	for _, r := range reasons {
		if strings.Contains(r, "would run") || strings.Contains(r, "no cache entry") {
			hasRun = true
			break
		}
	}
	if !hasRun {
		t.Errorf("expected would run due to changed steps, got %v", reasons)
	}
}

func TestRunWhenCmd(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "ran")
	gate := filepath.Join(dir, "gate")
	cfg := &config.File{
		RootDir: dir,
		Targets: []*config.Target{
			{
				Name:    "guarded",
				WhenCmd: []string{"sh", "-c", "test -f '" + strings.ReplaceAll(gate, "'", "'\\''") + "'"},
				Steps:   []config.Step{{Argv: []string{systemTouch(), marker}}},
			},
		},
	}
	// Gate missing: target skipped.
	err := Run(context.Background(), cfg, "guarded", RunOptions{RootDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Error("when cmd false: target should have been skipped")
	}
	// Create gate so when cmd succeeds.
	if err := os.WriteFile(filepath.Join(dir, "gate"), nil, 0644); err != nil {
		t.Fatal(err)
	}
	err = Run(context.Background(), cfg, "guarded", RunOptions{RootDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Errorf("when cmd true: expected marker, got %v", err)
	}
}

func TestWhyReasons(t *testing.T) {
	dir := t.TempDir()
	opts := RunOptions{RootDir: dir}

	t.Run("no inputs or outputs", func(t *testing.T) {
		tgt := &config.Target{Name: "always", Steps: []config.Step{{Argv: []string{"true"}}}}
		reasons, err := WhyReasons(dir, tgt, opts, nil)
		if err != nil {
			t.Fatal(err)
		}
		if len(reasons) != 1 || reasons[0] != "target has no inputs/outputs (always runs)" {
			t.Errorf("got %v", reasons)
		}
	})

	t.Run("no cache entry", func(t *testing.T) {
		os.WriteFile(filepath.Join(dir, "in"), []byte("x"), 0644)
		tgt := &config.Target{
			Name:    "build",
			Inputs:  []string{"in"},
			Outputs: []string{"out"},
			Steps:   []config.Step{{Argv: []string{"true"}}},
		}
		reasons, err := WhyReasons(dir, tgt, opts, nil)
		if err != nil {
			t.Fatal(err)
		}
		if len(reasons) != 1 || reasons[0] != "no cache entry (would run)" {
			t.Errorf("got %v", reasons)
		}
	})

	t.Run("up to date cache hit", func(t *testing.T) {
		inPath := filepath.Join(dir, "hit_in")
		outPath := filepath.Join(dir, "hit_out")
		os.WriteFile(inPath, []byte("data"), 0644)
		os.WriteFile(outPath, []byte("out"), 0644)
		cfg := &config.File{
			RootDir: dir,
			Targets: []*config.Target{{
				Name: "hit", Inputs: []string{"hit_in"}, Outputs: []string{"hit_out"},
				Steps: []config.Step{{Argv: []string{"true"}}},
			}},
		}
		// Run once to create manifest.
		if err := Run(context.Background(), cfg, "hit", opts); err != nil {
			t.Fatal(err)
		}
		reasons, err := WhyReasons(dir, cfg.TargetByName("hit"), opts, nil)
		if err != nil {
			t.Fatal(err)
		}
		if len(reasons) != 1 || reasons[0] != "up to date (cache hit); would skip" {
			t.Errorf("got %v", reasons)
		}
	})

	t.Run("outputs missing", func(t *testing.T) {
		sub := filepath.Join(dir, "missing_out")
		os.MkdirAll(sub, 0755)
		os.WriteFile(filepath.Join(sub, "in"), []byte("x"), 0644)
		tgt := &config.Target{
			Name: "missing", Inputs: []string{"in"}, Outputs: []string{"out"},
			Steps: []config.Step{{Argv: []string{"sh", "-c", "cp in out"}}},
		}
		// Run once so manifest is saved with output "out"; then remove out.
		Run(context.Background(), &config.File{RootDir: sub, Targets: []*config.Target{tgt}}, "missing", RunOptions{RootDir: sub})
		os.Remove(filepath.Join(sub, "out"))
		reasons, err := WhyReasons(sub, tgt, RunOptions{RootDir: sub}, nil)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, r := range reasons {
			if r == "outputs missing:" || r == "-> would run" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected outputs missing / would run, got %v", reasons)
		}
	})

	t.Run("inputs changed", func(t *testing.T) {
		sub := filepath.Join(dir, "changed")
		os.MkdirAll(sub, 0755)
		inPath := filepath.Join(sub, "in")
		os.WriteFile(inPath, []byte("v1"), 0644)
		os.WriteFile(filepath.Join(sub, "out"), []byte("o"), 0644)
		tgt := &config.Target{
			Name: "changed", Inputs: []string{"in"}, Outputs: []string{"out"},
			Steps: []config.Step{{Argv: []string{"true"}}},
		}
		Run(context.Background(), &config.File{RootDir: sub, Targets: []*config.Target{tgt}}, "changed", RunOptions{RootDir: sub})
		os.WriteFile(inPath, []byte("v2"), 0644)
		reasons, err := WhyReasons(sub, tgt, RunOptions{RootDir: sub}, nil)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, r := range reasons {
			if r == "-> would run" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected would run (inputs changed), got %v", reasons)
		}
	})

	t.Run("inputs changed with prev manifest from LoadManifestForTarget", func(t *testing.T) {
		sub := t.TempDir()
		os.WriteFile(filepath.Join(sub, "in"), []byte("newcontent"), 0644)
		tgt := &config.Target{
			Name: "t", Inputs: []string{"in"}, Outputs: []string{"out"},
			Steps: []config.Step{{Argv: []string{"true"}}},
		}
		oldKey := cache.Key("t", []byte("oldhash"), []byte("oldsig"))
		oldManifest := &cache.Manifest{TargetName: "t", InputHash: "old", InputHashes: map[string]string{"in": "old"}, OutputPaths: []string{"out"}}
		if err := cache.SaveManifest(sub, oldKey, oldManifest); err != nil {
			t.Fatal(err)
		}
		reasons, err := WhyReasons(sub, tgt, RunOptions{RootDir: sub}, nil)
		if err != nil {
			t.Fatal(err)
		}
		hasInputsChanged := false
		for _, r := range reasons {
			if strings.Contains(r, "inputs changed") {
				hasInputsChanged = true
				break
			}
		}
		if !hasInputsChanged {
			t.Errorf("expected inputs changed (prev from LoadManifestForTarget), got %v", reasons)
		}
	})
}
