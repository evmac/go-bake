package runner

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/evmac/go-bake/internal/cache"
	"github.com/evmac/go-bake/internal/config"
	"github.com/evmac/go-bake/internal/env"
	"github.com/evmac/go-bake/internal/resolve"
)

// RunOptions configures a run (cwd, env, passthrough argv, template data for expansion).
type RunOptions struct {
	RootDir      string
	Dotenv       []string
	TargetEnv    map[string]string
	CLIEnv       map[string]string // from --set env.FOO=bar; merged after TargetEnv
	DeclaredArgs map[string]string // for {{.argName}}
	LiveArgs     map[string]string // for {{.live.key}}
	Passthrough  []string          // raw args after "--" to append to passthrough step
	ShowCmd      bool              // print each command to stderr before running (BAKE_SHOW_CMD or --show-cmd)
}

// Run builds the DAG, runs dependencies in order, then runs the target's steps.
// Targets with inputs/outputs use the incremental cache and may be skipped when up to date.
func Run(ctx context.Context, cfg *config.File, targetName string, opts RunOptions) error {
	tgt := cfg.TargetByName(targetName)
	if tgt == nil {
		return fmt.Errorf("unknown target %q", targetName)
	}
	order, err := TopoOrder(cfg, targetName)
	if err != nil {
		return err
	}
	dotenvMap, err := env.LoadDotenv(opts.RootDir, cfg.Dotenv)
	if err != nil {
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
		depOpts := RunOptions{RootDir: opts.RootDir, Dotenv: opts.Dotenv, TargetEnv: dep.Env, CLIEnv: opts.CLIEnv}
		if err := runTargetWithCache(ctx, dep, opts.RootDir, dotenvMap, depOpts); err != nil {
			return fmt.Errorf("dep %q: %w", name, err)
		}
	}
	return runTargetWithCache(ctx, tgt, opts.RootDir, dotenvMap, opts)
}

// runTargetWithCache runs the target, skipping if incremental cache says up to date.
func runTargetWithCache(ctx context.Context, tgt *config.Target, rootDir string, dotenvMap map[string]string, opts RunOptions) error {
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
			return nil // condition false: skip target
		}
	} else if tgt.WhenEnv != "" {
		if os.Getenv(tgt.WhenEnv) == "" {
			return nil // condition false: skip target
		}
	}

	if len(tgt.Inputs) == 0 || len(tgt.Outputs) == 0 {
		return runTargetSteps(ctx, tgt, rootDir, dotenvMap, opts)
	}
	data := resolve.TemplateData(opts.DeclaredArgs, opts.LiveArgs)
	expandedInputs, _ := resolve.ExpandArgv(append([]string{}, tgt.Inputs...), data)
	expandedOutputs, _ := resolve.ExpandArgv(append([]string{}, tgt.Outputs...), data)
	inputFiles, err := cache.ResolveGlobs(rootDir, expandedInputs)
	if err != nil {
		return err
	}
	outputPaths, err := cache.ResolveGlobs(rootDir, expandedOutputs)
	if err != nil {
		return err
	}
	inputHash, inputHashesMap, err := cache.HashFilesMap(rootDir, inputFiles)
	if err != nil {
		return err
	}
	stepSig := stepSignature(tgt, rootDir, dotenvMap, opts)
	key := cache.Key(tgt.Name, inputHash, stepSig)
	manifest, err := cache.LoadManifest(rootDir, key)
	if err != nil {
		return err
	}
	inputHashHex := fmt.Sprintf("%x", inputHash)
	if manifest != nil && manifest.InputHash == inputHashHex {
		ok, _ := cache.OutputsExist(rootDir, manifest.OutputPaths)
		if ok {
			return nil
		}
	}
	if err := runTargetSteps(ctx, tgt, rootDir, dotenvMap, opts); err != nil {
		return err
	}
	outputPaths, _ = cache.ResolveGlobs(rootDir, expandedOutputs)
	mtimes, _ := cache.RecordOutputMTimes(rootDir, outputPaths)
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
	data := resolve.TemplateData(opts.DeclaredArgs, opts.LiveArgs)
	mergedEnv := env.Merge(os.Environ(), dotenvMap, targetEnv, opts.CLIEnv)
	if len(data) > 0 && len(targetEnv) > 0 {
		exp, _ := resolve.ExpandEnv(targetEnv, data)
		mergedEnv = env.Merge(os.Environ(), dotenvMap, exp, opts.CLIEnv)
	}
	h := sha256.New()
	for i, step := range tgt.Steps {
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
	passthroughStep := tgt.PassthroughStep
	if passthroughStep <= 0 && len(tgt.Steps) > 0 {
		passthroughStep = len(tgt.Steps)
	}
	for i, step := range tgt.Steps {
		argv := append([]string{}, step.Argv...)
		if len(data) > 0 {
			exp, err := resolve.ExpandArgv(argv, data)
			if err != nil {
				return fmt.Errorf("step %d: %w", i+1, err)
			}
			argv = exp
		}
		if len(opts.Passthrough) > 0 && i+1 == passthroughStep {
			argv = append(argv, opts.Passthrough...)
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
		if err := runStep(ctx, step.Runner, argv, stepCwd, stepEnv); err != nil {
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

func runStep(ctx context.Context, runner string, argv []string, cwd string, envMap map[string]string) error {
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
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
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
