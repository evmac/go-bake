package lifecycle

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/evmac/go-bake/internal/config"
	"github.com/evmac/go-bake/internal/env"
	"github.com/evmac/go-bake/internal/runner/container"
)

const workspaceMountPath = "/workspace"

// StartDaemonOpts optionally provides a container backend and registry for testing (avoids needing Docker).
// When nil, StartDaemon uses the default Docker client when the daemon has an image.
type StartDaemonOpts struct {
	Backend  container.DockerBackend
	Registry container.NetVolRegistry
}

// StartDaemon starts the daemon's first step: on the host (no image) as a detached process, or in a container when d.Image is set.
// Returns (pid, containerID, error). For host daemons containerID is ""; for container daemons pid is 0.
// envOverlay is merged on top of daemon env (e.g. profile); can be nil. testOpts is for tests only; pass nil in production.
func StartDaemon(ctx context.Context, rootDir string, d *config.Daemon, dotenvMap map[string]string, cliEnv map[string]string, envOverlay map[string]string, testOpts *StartDaemonOpts) (pid int, containerID string, err error) {
	if len(d.Steps) == 0 {
		return 0, "", fmt.Errorf("daemon %q has no steps", d.Name)
	}
	step := d.Steps[0]
	daemonEnv := d.Env
	if len(envOverlay) > 0 {
		if daemonEnv == nil {
			daemonEnv = make(map[string]string)
		} else {
			copied := make(map[string]string, len(daemonEnv)+len(envOverlay))
			for k, v := range daemonEnv {
				copied[k] = v
			}
			daemonEnv = copied
		}
		for k, v := range envOverlay {
			daemonEnv[k] = v
		}
	}
	mergedEnv := env.Merge(os.Environ(), dotenvMap, daemonEnv, cliEnv)

	if d.Image != "" && !d.Unsafe {
		workingDir := workspaceMountPath
		if d.Cwd != "" {
			workingDir = workspaceMountPath + "/" + filepath.ToSlash(d.Cwd)
		}
		opts := container.DaemonContainerOptions{
			Image:      d.Image,
			RootDir:    rootDir,
			TargetCwd:  d.Cwd,
			Networks:   d.Networks,
			Volumes:    d.Volumes,
			Cmd:        step.Argv,
			Env:        mergedEnv,
			WorkingDir: workingDir,
		}
		if testOpts != nil && testOpts.Backend != nil {
			opts.Backend = testOpts.Backend
			opts.Registry = testOpts.Registry
		}
		id, err := container.StartDaemonContainer(ctx, opts)
		if err != nil {
			return 0, "", fmt.Errorf("start daemon %q in container: %w", d.Name, err)
		}
		return 0, id, nil
	}

	cwd := rootDir
	if d.Cwd != "" {
		cwd = filepath.Join(rootDir, d.Cwd)
	}
	argv := step.Argv
	var cmd *exec.Cmd
	if step.Runner != "" {
		script := joinArgv(argv)
		cmd = exec.Command(step.Runner, "-c", script)
	} else {
		if len(argv) == 0 {
			return 0, "", fmt.Errorf("daemon %q step has empty argv", d.Name)
		}
		cmd = exec.Command(argv[0], argv[1:]...)
	}
	cmd.Dir = cwd
	cmd.Env = envMapToSlice(mergedEnv)
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	setDetached(cmd)
	if err := cmd.Start(); err != nil {
		return 0, "", fmt.Errorf("start daemon %q: %w", d.Name, err)
	}
	pid = cmd.Process.Pid
	cmd.Process.Release()
	return pid, "", nil
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
