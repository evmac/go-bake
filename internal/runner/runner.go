package runner

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/evmac/go-bake/internal/cache"
	"github.com/evmac/go-bake/internal/config"
	"github.com/evmac/go-bake/internal/env"
	"github.com/evmac/go-bake/internal/resolve"
	"github.com/evmac/go-bake/internal/runner/container"
)

// RunEvent is one NDJSON line for machine-readable output (--json).
type RunEvent struct {
	Event      string `json:"event"`
	Ts         string `json:"ts,omitempty"`
	Target     string `json:"target,omitempty"`
	Step       int    `json:"step,omitempty"`
	DurationMs int64  `json:"duration_ms,omitempty"`
	Ok         bool   `json:"ok,omitempty"`
	Message    string `json:"message,omitempty"`
}

// RunOptions configures a run (cwd, env, passthrough argv, template data for expansion).
type RunOptions struct {
	RootDir           string
	Dotenv            []string
	TargetEnv         map[string]string
	CLIEnv            map[string]string                           // from --set env.FOO=bar; merged after TargetEnv
	DeclaredArgs      map[string]string                           // for {{.argName}}
	LiveArgs          map[string]string                           // for {{.live.key}}
	PassthroughByStep map[int][]string                            // step index (1-based) -> args after "--"; when multiple slots, split by "--"
	Preset            *config.Preset                              // optional named preset (extra argv + env overlay)
	ShowCmd           bool                                        // print each command to stderr before running (BAKE_SHOW_CMD or --show-cmd)
	EventWriter       io.Writer                                   // when set, emit NDJSON run events (for --json)
	StepStdout        io.Writer                                   // when set (e.g. for --json), step stdout goes here instead of os.Stdout
	StepStderr        io.Writer                                   // when set (e.g. for --json), step stderr goes here instead of os.Stderr
	MaxParallel       int                                         // max targets running at once (0 or 1 = sequential; >1 = parallel by level)
	PoolSems          *sync.Map                                   // optional: name -> chan struct{} (semaphore per pool name)
	Mutexes           *sync.Map                                   // optional: name -> *sync.Mutex (mutex per name)
	TimingReporter    func(target string, duration time.Duration) // optional: called when a target finishes (for --timing)
	ArtifactReporter  func(target string, paths []string)         // optional: called with output paths after a target runs (for --artifacts)
	NetVolRegistry container.NetVolRegistry // optional: for containerized targets (image); first reference creates net/vol
}

func emitEvent(w io.Writer, e RunEvent) {
	if w == nil {
		return
	}
	e.Ts = time.Now().UTC().Format(time.RFC3339Nano)
	b, _ := json.Marshal(e)
	w.Write(b)
	w.Write([]byte{'\n'})
}

// Run builds the DAG, runs dependencies in order, then runs the target's steps.
// When MaxParallel > 1, targets in the same level run concurrently (up to MaxParallel).
func Run(ctx context.Context, cfg *config.File, targetName string, opts RunOptions) error {
	ev := opts.EventWriter
	if ev != nil {
		emitEvent(ev, RunEvent{Event: "run_start", Target: targetName})
	}
	tgt := cfg.TargetByName(targetName)
	if tgt == nil {
		if ev != nil {
			emitEvent(ev, RunEvent{Event: "run_end", Ok: false, Message: "unknown target"})
		}
		return fmt.Errorf("unknown target %q", targetName)
	}
	dotenvMap, err := env.LoadDotenv(opts.RootDir, cfg.Dotenv)
	if err != nil {
		if ev != nil {
			emitEvent(ev, RunEvent{Event: "run_end", Ok: false, Message: err.Error()})
		}
		return err
	}
	// Shared registry for pool semaphores and mutexes (so parallel runs can coordinate).
	if opts.PoolSems == nil {
		opts.PoolSems = &sync.Map{}
	}
	if opts.Mutexes == nil {
		opts.Mutexes = &sync.Map{}
	}
	// Optional Docker net/vol registry for containerized targets (image); create once per run.
	if opts.NetVolRegistry == nil {
		if reg, closeFn, err := container.NewDockerRegistryFromEnv(); err == nil {
			opts.NetVolRegistry = reg
			defer closeFn()
		}
	}
	if opts.MaxParallel <= 1 {
		// Sequential: original behavior
		order, err := TopoOrder(cfg, targetName)
		if err != nil {
			if ev != nil {
				emitEvent(ev, RunEvent{Event: "run_end", Ok: false, Message: err.Error()})
			}
			return err
		}
		for _, name := range order {
			if name == targetName {
				break
			}
			dep := cfg.TargetByName(name)
			if dep == nil {
				continue
			}
			depOpts := RunOptions{RootDir: opts.RootDir, Dotenv: opts.Dotenv, TargetEnv: dep.Env, CLIEnv: opts.CLIEnv, EventWriter: ev}
			if err := runTargetWithCache(ctx, dep, opts.RootDir, dotenvMap, depOpts); err != nil {
				if ev != nil {
					emitEvent(ev, RunEvent{Event: "run_end", Ok: false, Message: err.Error()})
				}
				return fmt.Errorf("dep %q: %w", name, err)
			}
		}
		err = runTargetWithCache(ctx, tgt, opts.RootDir, dotenvMap, opts)
		if ev != nil {
			emitEvent(ev, RunEvent{Event: "run_end", Ok: err == nil, Message: errMsg(err)})
		}
		return err
	}
	// Parallel by level
	levels, err := LevelOrder(cfg, targetName)
	if err != nil {
		if ev != nil {
			emitEvent(ev, RunEvent{Event: "run_end", Ok: false, Message: err.Error()})
		}
		return err
	}
	var sem = make(chan struct{}, opts.MaxParallel)
	var firstErr error
	var mu sync.Mutex
	for _, level := range levels {
		var wg sync.WaitGroup
		for _, name := range level {
			dep := cfg.TargetByName(name)
			if dep == nil {
				continue
			}
			runOpts := opts
			if name != targetName {
				runOpts = RunOptions{RootDir: opts.RootDir, Dotenv: opts.Dotenv, TargetEnv: dep.Env, CLIEnv: opts.CLIEnv, EventWriter: ev, MaxParallel: opts.MaxParallel, PoolSems: opts.PoolSems, Mutexes: opts.Mutexes, NetVolRegistry: opts.NetVolRegistry}
			}
			wg.Add(1)
			go func(n string, d *config.Target, ro RunOptions) {
				defer wg.Done()
				select {
				case sem <- struct{}{}:
					defer func() { <-sem }()
				case <-ctx.Done():
					mu.Lock()
					if firstErr == nil {
						firstErr = ctx.Err()
					}
					mu.Unlock()
					return
				}
				if err := runTargetWithCache(ctx, d, opts.RootDir, dotenvMap, ro); err != nil {
					mu.Lock()
					if firstErr == nil {
						firstErr = fmt.Errorf("%s: %w", n, err)
					}
					mu.Unlock()
				}
			}(name, dep, runOpts)
		}
		wg.Wait()
		if firstErr != nil {
			if ev != nil {
				emitEvent(ev, RunEvent{Event: "run_end", Ok: false, Message: firstErr.Error()})
			}
			return firstErr
		}
	}
	if ev != nil {
		emitEvent(ev, RunEvent{Event: "run_end", Ok: true})
	}
	return nil
}

func errMsg(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func getPoolSem(sm *sync.Map, name string) chan struct{} {
	v, _ := sm.LoadOrStore(name, make(chan struct{}, 1))
	return v.(chan struct{})
}

func getMutex(m *sync.Map, name string) *sync.Mutex {
	v, _ := m.LoadOrStore(name, &sync.Mutex{})
	return v.(*sync.Mutex)
}

// runTargetWithCache runs the target, skipping if incremental cache says up to date.
func runTargetWithCache(ctx context.Context, tgt *config.Target, rootDir string, dotenvMap map[string]string, opts RunOptions) error {
	if tgt.Pool != "" && opts.PoolSems != nil {
		sem := getPoolSem(opts.PoolSems, tgt.Pool)
		select {
		case sem <- struct{}{}:
			defer func() { <-sem }()
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if tgt.Mutex != "" && opts.Mutexes != nil {
		mu := getMutex(opts.Mutexes, tgt.Mutex)
		mu.Lock()
		defer mu.Unlock()
	}
	start := time.Now()
	defer func() {
		if opts.TimingReporter != nil {
			opts.TimingReporter(tgt.Name, time.Since(start))
		}
	}()
	ev := opts.EventWriter
	if ev != nil {
		emitEvent(ev, RunEvent{Event: "target_start", Target: tgt.Name})
	}
	// When guard: skip target (no-op, success for DAG) if condition is false.
	targetEnv := opts.TargetEnv
	if targetEnv == nil {
		targetEnv = tgt.Env
	}
	if len(tgt.WhenCmd) > 0 {
		mergedEnv := env.Merge(os.Environ(), dotenvMap, targetEnv, opts.CLIEnv)
		mergedEnv = stripBakeBinFromPath(mergedEnv, rootDir)
		cwd := rootDir
		if tgt.Cwd != "" {
			cwd = filepath.Join(rootDir, tgt.Cwd)
		}
		if opts.ShowCmd {
			printCmdToStderr("", tgt.WhenCmd)
		}
		cmd := exec.CommandContext(ctx, tgt.WhenCmd[0], tgt.WhenCmd[1:]...)
		cmd.Dir = cwd
		cmd.Env = envMapToSlice(mergedEnv)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			if ev != nil {
				emitEvent(ev, RunEvent{Event: "when_skip", Target: tgt.Name, Ok: true})
			}
			return nil // condition false: skip target
		}
	} else if tgt.WhenEnv != "" {
		if os.Getenv(tgt.WhenEnv) == "" {
			if ev != nil {
				emitEvent(ev, RunEvent{Event: "when_skip", Target: tgt.Name, Ok: true})
			}
			return nil // condition false: skip target
		}
	}

	if len(tgt.Inputs) == 0 || len(tgt.Outputs) == 0 {
		err := runTargetSteps(ctx, tgt, rootDir, dotenvMap, opts)
		if ev != nil {
			emitEvent(ev, RunEvent{Event: "target_end", Target: tgt.Name, Ok: err == nil, Message: errMsg(err)})
		}
		return err
	}
	data := resolve.TemplateData(opts.DeclaredArgs, opts.LiveArgs)
	expandedInputs, _ := resolve.ExpandArgv(append([]string{}, tgt.Inputs...), data)
	expandedOutputs, _ := resolve.ExpandArgv(append([]string{}, tgt.Outputs...), data)
	inputFiles, err := cache.ResolveGlobs(rootDir, expandedInputs)
	if err != nil {
		if ev != nil {
			emitEvent(ev, RunEvent{Event: "target_end", Target: tgt.Name, Ok: false, Message: err.Error()})
		}
		return err
	}
	outputPaths, err := cache.ResolveGlobs(rootDir, expandedOutputs)
	if err != nil {
		if ev != nil {
			emitEvent(ev, RunEvent{Event: "target_end", Target: tgt.Name, Ok: false, Message: err.Error()})
		}
		return err
	}
	inputHash, inputHashesMap, err := cache.HashFilesMap(rootDir, inputFiles)
	if err != nil {
		if ev != nil {
			emitEvent(ev, RunEvent{Event: "target_end", Target: tgt.Name, Ok: false, Message: err.Error()})
		}
		return err
	}
	stepSig := stepSignature(tgt, rootDir, dotenvMap, opts)
	key := cache.Key(tgt.Name, inputHash, stepSig)
	manifest, err := cache.LoadManifest(rootDir, key)
	if err != nil {
		if ev != nil {
			emitEvent(ev, RunEvent{Event: "target_end", Target: tgt.Name, Ok: false, Message: err.Error()})
		}
		return err
	}
	inputHashHex := fmt.Sprintf("%x", inputHash)
	if manifest != nil && manifest.InputHash == inputHashHex {
		ok, _ := cache.OutputsExist(rootDir, manifest.OutputPaths)
		if ok {
			if ev != nil {
				emitEvent(ev, RunEvent{Event: "cache_skip", Target: tgt.Name, Ok: true})
			}
			return nil
		}
	}
	if err := runTargetSteps(ctx, tgt, rootDir, dotenvMap, opts); err != nil {
		if ev != nil {
			emitEvent(ev, RunEvent{Event: "target_end", Target: tgt.Name, Ok: false, Message: err.Error()})
		}
		return err
	}
	outputPaths, _ = cache.ResolveGlobs(rootDir, expandedOutputs)
	mtimes, _ := cache.RecordOutputMTimes(rootDir, outputPaths)
	if opts.ArtifactReporter != nil && len(outputPaths) > 0 {
		opts.ArtifactReporter(tgt.Name, outputPaths)
	}
	if ev != nil {
		emitEvent(ev, RunEvent{Event: "target_end", Target: tgt.Name, Ok: true})
	}
	return cache.SaveManifest(rootDir, key, &cache.Manifest{
		TargetName:   tgt.Name,
		InputHash:    inputHashHex,
		InputHashes:  inputHashesMap,
		OutputPaths:  outputPaths,
		OutputMTimes: mtimes,
	})
}

// stepSignature returns a hash of the target's expanded steps (argv + env) for cache keying.
func stepSignature(tgt *config.Target, rootDir string, dotenvMap map[string]string, opts RunOptions) []byte {
	targetEnv := opts.TargetEnv
	if targetEnv == nil {
		targetEnv = tgt.Env
	}
	if opts.Preset != nil && len(opts.Preset.Env) > 0 {
		if targetEnv == nil {
			targetEnv = make(map[string]string)
		} else {
			copied := make(map[string]string)
			for k, v := range targetEnv {
				copied[k] = v
			}
			targetEnv = copied
		}
		for k, v := range opts.Preset.Env {
			targetEnv[k] = v
		}
	}
	data := resolve.TemplateData(opts.DeclaredArgs, opts.LiveArgs)
	mergedEnv := env.Merge(os.Environ(), dotenvMap, targetEnv, opts.CLIEnv)
	if len(data) > 0 && len(targetEnv) > 0 {
		exp, _ := resolve.ExpandEnv(targetEnv, data)
		mergedEnv = env.Merge(os.Environ(), dotenvMap, exp, opts.CLIEnv)
	}
	h := sha256.New()
	stepsForSig := tgt.Steps
	if opts.Preset != nil {
		h.Write([]byte("preset:" + opts.Preset.Name))
		if len(opts.Preset.Steps) > 0 {
			stepsForSig = opts.Preset.Steps
		} else {
			for _, a := range opts.Preset.Argv {
				h.Write([]byte(a))
			}
		}
	}
	for i, step := range stepsForSig {
		argv := append([]string{}, step.Argv...)
		if len(data) > 0 {
			argv, _ = resolve.ExpandArgv(argv, data)
		}
		h.Write([]byte(fmt.Sprintf("step:%d", i)))
		for _, a := range argv {
			h.Write([]byte(a))
		}
		if len(step.Env) > 0 {
			keys := make([]string, 0, len(step.Env))
			for k := range step.Env {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				v := step.Env[k]
				if len(data) > 0 {
					exp, _ := resolve.ExpandEnv(map[string]string{k: v}, data)
					v = exp[k]
				}
				h.Write([]byte(k))
				h.Write([]byte(v))
			}
		}
	}
	envSlice := envMapToSlice(mergedEnv)
	sort.Strings(envSlice)
	for _, s := range envSlice {
		h.Write([]byte(s))
	}
	return h.Sum(nil)
}

func runTargetSteps(ctx context.Context, tgt *config.Target, rootDir string, dotenvMap map[string]string, opts RunOptions) error {
	targetEnv := opts.TargetEnv
	if targetEnv == nil {
		targetEnv = tgt.Env
	}
	// Apply preset env overlay if present
	if opts.Preset != nil && len(opts.Preset.Env) > 0 {
		if targetEnv == nil {
			targetEnv = make(map[string]string)
		} else {
			copied := make(map[string]string)
			for k, v := range targetEnv {
				copied[k] = v
			}
			targetEnv = copied
		}
		for k, v := range opts.Preset.Env {
			targetEnv[k] = v
		}
	}
	data := resolve.TemplateData(opts.DeclaredArgs, opts.LiveArgs)
	mergedEnv := env.Merge(os.Environ(), dotenvMap, targetEnv, opts.CLIEnv)
	// Expand target-level env if we have template data
	if len(data) > 0 && len(targetEnv) > 0 {
		exp, err := resolve.ExpandEnv(targetEnv, data)
		if err != nil {
			return err
		}
		mergedEnv = env.Merge(os.Environ(), dotenvMap, exp, opts.CLIEnv)
	}
	mergedEnv = stripBakeBinFromPath(mergedEnv, rootDir)
	cwd := rootDir
	if tgt.Cwd != "" {
		cwd = filepath.Join(rootDir, tgt.Cwd)
	}
	steps := tgt.Steps
	passthroughStep := tgt.PassthroughStep
	usePresetArgv := opts.Preset != nil && len(opts.Preset.Argv) > 0
	if opts.Preset != nil && len(opts.Preset.Steps) > 0 {
		steps = opts.Preset.Steps
		usePresetArgv = false // preset steps replace target steps; no argv append
	}
	if len(tgt.Passthrough) > 0 {
		// Use first slot for default when no multi-slot; runner uses PassthroughByStep per step
		passthroughStep = tgt.Passthrough[0].Step
	}
	if passthroughStep <= 0 && len(steps) > 0 {
		passthroughStep = len(steps)
	}

	// Container path: run all steps inside a single container when image is set and not unsafe.
	if tgt.Image != "" && !tgt.Unsafe {
		if opts.NetVolRegistry == nil {
			return fmt.Errorf("Docker is required for target %q with image %q (install Docker or set DOCKER_HOST)", tgt.Name, tgt.Image)
		}
		var execSteps []container.ExecStep
		for i, step := range steps {
			argv := append([]string{}, step.Argv...)
			if len(data) > 0 {
				exp, err := resolve.ExpandArgv(argv, data)
				if err != nil {
					return fmt.Errorf("step %d: %w", i+1, err)
				}
				argv = exp
			}
			stepNum := i + 1
			if usePresetArgv && stepNum == passthroughStep {
				argv = append(argv, opts.Preset.Argv...)
			}
			if opts.PassthroughByStep != nil && len(opts.PassthroughByStep[stepNum]) > 0 {
				argv = append(argv, opts.PassthroughByStep[stepNum]...)
			}
			if len(argv) == 0 {
				continue
			}
			if step.Runner != "" {
				argv = append([]string{step.Runner}, argv...)
			}
			stepEnv := mergedEnv
			if len(step.Env) > 0 {
				stepEnv = make(map[string]string)
				for k, v := range mergedEnv {
					stepEnv[k] = v
				}
				toMerge := step.Env
				if len(data) > 0 {
					exp, err := resolve.ExpandEnv(step.Env, data)
					if err != nil {
						return fmt.Errorf("step %d env: %w", i+1, err)
					}
					toMerge = exp
				}
				for k, v := range toMerge {
					stepEnv[k] = v
				}
			}
			stepCwd := cwd
			if step.Cwd != "" {
				stepCwd = filepath.Join(rootDir, step.Cwd)
			}
			execSteps = append(execSteps, container.ExecStep{Argv: argv, Env: stepEnv, WorkingDir: stepCwd})
		}
		var stepOut, stepErr io.Writer = os.Stdout, os.Stderr
		if opts.StepStdout != nil {
			stepOut = opts.StepStdout
		}
		if opts.StepStderr != nil {
			stepErr = opts.StepStderr
		}
		err := container.Run(ctx, container.RunOptions{
			Image:     tgt.Image,
			RootDir:   rootDir,
			TargetCwd: tgt.Cwd,
			Networks:  tgt.Networks,
			Volumes:   tgt.Volumes,
			Steps:     execSteps,
			Registry:  opts.NetVolRegistry,
			Stdout:    stepOut,
			Stderr:    stepErr,
			ShowCmd:   opts.ShowCmd,
		})
		return err
	}

	// Host path: run each step via runStep.
	for i, step := range steps {
		argv := append([]string{}, step.Argv...)
		if len(data) > 0 {
			exp, err := resolve.ExpandArgv(argv, data)
			if err != nil {
				return fmt.Errorf("step %d: %w", i+1, err)
			}
			argv = exp
		}
		stepNum := i + 1
		if usePresetArgv && stepNum == passthroughStep {
			argv = append(argv, opts.Preset.Argv...)
		}
		if opts.PassthroughByStep != nil && len(opts.PassthroughByStep[stepNum]) > 0 {
			argv = append(argv, opts.PassthroughByStep[stepNum]...)
		}
		if len(argv) == 0 {
			continue
		}
		stepEnv := mergedEnv
		if len(step.Env) > 0 {
			stepEnv = make(map[string]string)
			for k, v := range mergedEnv {
				stepEnv[k] = v
			}
			toMerge := step.Env
			if len(data) > 0 {
				exp, err := resolve.ExpandEnv(step.Env, data)
				if err != nil {
					return fmt.Errorf("step %d env: %w", i+1, err)
				}
				toMerge = exp
			}
			for k, v := range toMerge {
				stepEnv[k] = v
			}
		}
		stepCwd := cwd
		if step.Cwd != "" {
			stepCwd = filepath.Join(rootDir, step.Cwd)
		}
		if opts.ShowCmd {
			printCmdToStderr(step.Runner, argv)
		}
		if opts.EventWriter != nil {
			emitEvent(opts.EventWriter, RunEvent{Event: "step_start", Target: tgt.Name, Step: stepNum})
		}
		var stepOut, stepErr io.Writer = os.Stdout, os.Stderr
		if opts.StepStdout != nil {
			stepOut = opts.StepStdout
		}
		if opts.StepStderr != nil {
			stepErr = opts.StepStderr
		}
		start := time.Now()
		err := runStep(ctx, step.Runner, argv, stepCwd, stepEnv, stepOut, stepErr)
		if opts.EventWriter != nil {
			emitEvent(opts.EventWriter, RunEvent{
				Event:      "step_end",
				Target:     tgt.Name,
				Step:       stepNum,
				DurationMs: time.Since(start).Milliseconds(),
				Ok:         err == nil,
				Message:    errMsg(err),
			})
		}
		if err != nil {
			return fmt.Errorf("step %d: %w", i+1, err)
		}
	}
	return nil
}

// stripBakeBinFromPath returns a copy of envMap with this project's .bake/bin removed
// from PATH so steps run the real system binaries (e.g. test, act) instead of shims.
func stripBakeBinFromPath(envMap map[string]string, rootDir string) map[string]string {
	bakeBin := filepath.Clean(filepath.Join(rootDir, ".bake", "bin"))
	pathVal, ok := envMap["PATH"]
	if !ok || pathVal == "" {
		return envMap
	}
	parts := filepath.SplitList(pathVal)
	var kept []string
	for _, p := range parts {
		if filepath.Clean(p) != bakeBin {
			kept = append(kept, p)
		}
	}
	if len(kept) == len(parts) {
		return envMap
	}
	out := make(map[string]string, len(envMap))
	for k, v := range envMap {
		out[k] = v
	}
	out["PATH"] = strings.Join(kept, string(filepath.ListSeparator))
	return out
}

// printCmdToStderr writes the command line to os.Stderr (e.g. for --show-cmd / BAKE_SHOW_CMD).
func printCmdToStderr(runner string, argv []string) {
	line := joinArgv(argv)
	if runner != "" {
		fmt.Fprintf(os.Stderr, "+ %s -c %s\n", runner, line)
	} else {
		fmt.Fprintf(os.Stderr, "+ %s\n", line)
	}
}

func runStep(ctx context.Context, runner string, argv []string, cwd string, envMap map[string]string, stdout, stderr io.Writer) error {
	if stdout == nil {
		stdout = os.Stdout
	}
	if stderr == nil {
		stderr = os.Stderr
	}
	var cmd *exec.Cmd
	if runner != "" {
		// shell: runner -c "argv[0] argv[1] ..." (simplified: join with space)
		script := joinArgv(argv)
		cmd = exec.CommandContext(ctx, runner, "-c", script)
	} else {
		cmd = exec.CommandContext(ctx, argv[0], argv[1:]...)
	}
	cmd.Dir = cwd
	cmd.Env = envMapToSlice(envMap)
	cmd.Stdin = os.Stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	setProcessGroup(cmd)
	if err := cmd.Run(); err != nil {
		return err
	}
	return nil
}

func joinArgv(argv []string) string {
	var b []byte
	for i, a := range argv {
		if i > 0 {
			b = append(b, ' ')
		}
		b = append(b, quoteForShell(a)...)
	}
	return string(b)
}

func quoteForShell(s string) []byte {
	for _, c := range s {
		if c == ' ' || c == '\'' || c == '"' || c == '\\' {
			return append(append([]byte{'\''}, escapeSingleQuoted(s)...), '\'')
		}
	}
	return []byte(s)
}

func escapeSingleQuoted(s string) []byte {
	var b []byte
	for i := 0; i < len(s); i++ {
		if s[i] == '\'' {
			b = append(b, '\'', '\\', '\'', '\'')
		} else {
			b = append(b, s[i])
		}
	}
	return b
}

func envMapToSlice(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k, v := range m {
		out = append(out, k+"="+v)
	}
	return out
}
