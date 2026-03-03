package runner

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/evmac/go-bake/internal/config"
	"github.com/evmac/go-bake/internal/env"
	"github.com/evmac/go-bake/internal/resolve"
)

// RunOptions configures a run (cwd, env, passthrough argv, template data for expansion).
type RunOptions struct {
	RootDir      string
	Dotenv       []string
	TargetEnv    map[string]string
	DeclaredArgs map[string]string   // for {{.argName}}
	LiveArgs     map[string]string   // for {{.live.key}}
	Passthrough  []string            // raw args after "--" to append to passthrough step
}

// Run builds the DAG, runs dependencies in order, then runs the target's steps.
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
		depOpts := RunOptions{RootDir: opts.RootDir, Dotenv: opts.Dotenv, TargetEnv: dep.Env}
		if err := runTargetSteps(ctx, dep, opts.RootDir, dotenvMap, depOpts); err != nil {
			return fmt.Errorf("dep %q: %w", name, err)
		}
	}
	return runTargetSteps(ctx, tgt, opts.RootDir, dotenvMap, opts)
}

func runTargetSteps(ctx context.Context, tgt *config.Target, rootDir string, dotenvMap map[string]string, opts RunOptions) error {
	data := resolve.TemplateData(opts.DeclaredArgs, opts.LiveArgs)
	mergedEnv := env.Merge(os.Environ(), dotenvMap, tgt.Env)
	// Expand target-level env if we have template data
	if len(data) > 0 && len(tgt.Env) > 0 {
		exp, err := resolve.ExpandEnv(tgt.Env, data)
		if err != nil {
			return err
		}
		mergedEnv = env.Merge(os.Environ(), dotenvMap, exp)
	}
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
		if err := runStep(ctx, step.Runner, argv, stepCwd, stepEnv); err != nil {
			return fmt.Errorf("step %d: %w", i+1, err)
		}
	}
	return nil
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
