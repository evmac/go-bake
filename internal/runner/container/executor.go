package container

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/evmac/go-bake/internal/config"
	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/client"
)

const (
	workspaceMountPath = "/workspace"
	pullPolicyEnv      = "BAKE_PULL"
	pullAlways         = "always"
	pullNever          = "never"
	pullIfNotPresent   = "if-not-present"
)

// NetVolRegistry creates or returns existing networks and volumes. First reference creates; later references reuse.
// Implementations must be safe for concurrent use when the same name is used from multiple targets.
type NetVolRegistry interface {
	EnsureNetwork(ctx context.Context, name string) (networkIDOrName string, err error)
	EnsureVolume(ctx context.Context, ref config.VolumeRef, rootDir string) (hostSource, containerTarget string, err error)
}

// ExecStep is a single step to run inside the container (argv, env, working dir already expanded).
type ExecStep struct {
	Argv       []string
	Env        map[string]string
	WorkingDir string // absolute path on host; will be mapped to container path
}

// RunOptions holds parameters for running a target's steps in a container.
type RunOptions struct {
	Image     string
	RootDir   string
	TargetCwd string // relative to RootDir; empty means RootDir
	Networks  []string
	Volumes   []config.VolumeRef
	Steps     []ExecStep
	Registry  NetVolRegistry
	Stdout    io.Writer
	Stderr    io.Writer
	ShowCmd   bool
	// Backend is the Docker API used by Run. When nil, a real client is created from env (requires Docker).
	// Set in tests to inject a fake for coverage without a daemon.
	Backend DockerBackend
}

// ExecAttachStream is the result of attaching to an exec; caller reads output then closes.
type ExecAttachStream interface {
	io.Reader
	Close()
}

// imagePullResponse is the minimal interface used after ImagePull (avoids requiring full client.ImagePullResponse in tests).
type imagePullResponse interface {
	Close() error
	Wait(context.Context) error
}

// DockerBackend is the minimal Docker API used by Run. Implemented by *client.Client via backendAdapter; tests use a fake.
type DockerBackend interface {
	ImageInspect(ctx context.Context, image string, opts ...client.ImageInspectOption) (client.ImageInspectResult, error)
	ImagePull(ctx context.Context, ref string, opts client.ImagePullOptions) (imagePullResponse, error)
	ContainerCreate(ctx context.Context, opts client.ContainerCreateOptions) (client.ContainerCreateResult, error)
	ContainerRemove(ctx context.Context, containerID string, opts client.ContainerRemoveOptions) (client.ContainerRemoveResult, error)
	ContainerStart(ctx context.Context, containerID string, opts client.ContainerStartOptions) (client.ContainerStartResult, error)
	NetworkConnect(ctx context.Context, networkID string, opts client.NetworkConnectOptions) (client.NetworkConnectResult, error)
	ExecCreate(ctx context.Context, containerID string, opts client.ExecCreateOptions) (client.ExecCreateResult, error)
	ExecAttach(ctx context.Context, execID string, opts client.ExecAttachOptions) (ExecAttachStream, error)
	ExecInspect(ctx context.Context, execID string, opts client.ExecInspectOptions) (client.ExecInspectResult, error)
}

// Run runs all steps in a container: create container with image, mount rootDir at /workspace, attach networks/volumes, exec each step.
func Run(ctx context.Context, opts RunOptions) error {
	if opts.Stdout == nil {
		opts.Stdout = os.Stdout
	}
	if opts.Stderr == nil {
		opts.Stderr = os.Stderr
	}
	var backend DockerBackend
	if opts.Backend != nil {
		backend = opts.Backend
	} else {
		cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
		if err != nil {
			return fmt.Errorf("Docker is required for target with image %q: %w", opts.Image, err)
		}
		defer cli.Close()
		backend = &backendAdapter{Client: cli}
	}

	// Optional: pull image
	if err := maybePullImage(ctx, backend, opts.Image); err != nil {
		return err
	}

	// Resolve networks and volumes via registry
	networkMode := ""
	if len(opts.Networks) > 0 {
		netName, err := opts.Registry.EnsureNetwork(ctx, opts.Networks[0])
		if err != nil {
			return fmt.Errorf("ensure network %q: %w", opts.Networks[0], err)
		}
		networkMode = netName
	}
	var mounts []mount.Mount
	rootAbs, err := filepath.Abs(opts.RootDir)
	if err != nil {
		return fmt.Errorf("resolve root dir: %w", err)
	}
	mounts = append(mounts, mount.Mount{Type: mount.TypeBind, Source: rootAbs, Target: workspaceMountPath})
	for _, ref := range opts.Volumes {
		src, tgt, err := opts.Registry.EnsureVolume(ctx, ref, opts.RootDir)
		if err != nil {
			return fmt.Errorf("ensure volume %q: %w", ref.Name, err)
		}
		if ref.HostPath != "" {
			mounts = append(mounts, mount.Mount{Type: mount.TypeBind, Source: src, Target: tgt})
		} else {
			mounts = append(mounts, mount.Mount{Type: mount.TypeVolume, Source: src, Target: tgt})
		}
	}

	hostConfig := &container.HostConfig{
		Mounts:      mounts,
		NetworkMode: container.NetworkMode(networkMode),
		AutoRemove:  true,
	}
	// Connect additional networks after create (Docker allows one NetworkMode at create time)
	var extraNetworks []string
	if len(opts.Networks) > 1 {
		extraNetworks = opts.Networks[1:]
	}

	// Container stays running so we can exec into it
	cfg := &container.Config{
		Image:      opts.Image,
		Cmd:        []string{"sleep", "infinity"},
		WorkingDir: workspaceMountPath,
		Tty:        false,
		OpenStdin:  false,
	}
	createOpts := client.ContainerCreateOptions{
		Config:     cfg,
		HostConfig: hostConfig,
	}
	resp, err := backend.ContainerCreate(ctx, createOpts)
	if err != nil {
		return fmt.Errorf("create container: %w", err)
	}
	containerID := resp.ID
	defer func() {
		_, _ = backend.ContainerRemove(context.Background(), containerID, client.ContainerRemoveOptions{Force: true})
	}()

	for i, netName := range extraNetworks {
		netID, err := opts.Registry.EnsureNetwork(ctx, netName)
		if err != nil {
			return fmt.Errorf("ensure network %q: %w", netName, err)
		}
		if _, err := backend.NetworkConnect(ctx, netID, client.NetworkConnectOptions{Container: containerID}); err != nil {
			return fmt.Errorf("connect network %q (index %d): %w", netName, i+1, err)
		}
	}

	if _, err := backend.ContainerStart(ctx, containerID, client.ContainerStartOptions{}); err != nil {
		return fmt.Errorf("start container: %w", err)
	}

	// Working dir inside container: /workspace[/targetCwd]
	containerCwd := workspaceMountPath
	if opts.TargetCwd != "" {
		containerCwd = filepath.Join(workspaceMountPath, filepath.ToSlash(opts.TargetCwd))
	}

	for i, step := range opts.Steps {
		if len(step.Argv) == 0 {
			continue
		}
		stepCwd := containerCwd
		if step.WorkingDir != "" {
			// step.WorkingDir is absolute on host; map to container
			rel, err := filepath.Rel(opts.RootDir, step.WorkingDir)
			if err != nil {
				stepCwd = filepath.Join(workspaceMountPath, filepath.ToSlash(step.WorkingDir))
			} else {
				stepCwd = filepath.Join(workspaceMountPath, filepath.ToSlash(rel))
			}
		}
		envSlice := envMapToSlice(step.Env)
		if opts.ShowCmd {
			fmt.Fprintf(opts.Stderr, "+ %s\n", strings.Join(step.Argv, " "))
		}
		execCfg := client.ExecCreateOptions{
			Cmd:          step.Argv,
			Env:          envSlice,
			WorkingDir:   stepCwd,
			AttachStderr: true,
			AttachStdout: true,
			AttachStdin:  false,
		}
		execResp, err := backend.ExecCreate(ctx, containerID, execCfg)
		if err != nil {
			return fmt.Errorf("step %d exec create: %w", i+1, err)
		}
		attachStream, err := backend.ExecAttach(ctx, execResp.ID, client.ExecAttachOptions{TTY: false})
		if err != nil {
			return fmt.Errorf("step %d exec attach: %w", i+1, err)
		}
		_, err = stdcopy.StdCopy(opts.Stdout, opts.Stderr, attachStream)
		attachStream.Close()
		if err != nil && err != io.EOF {
			return fmt.Errorf("step %d stream: %w", i+1, err)
		}
		inspect, err := backend.ExecInspect(ctx, execResp.ID, client.ExecInspectOptions{})
		if err != nil {
			return fmt.Errorf("step %d exec inspect: %w", i+1, err)
		}
		if inspect.ExitCode != 0 {
			return fmt.Errorf("step %d exited with code %d", i+1, inspect.ExitCode)
		}
	}
	return nil
}

func maybePullImage(ctx context.Context, backend DockerBackend, image string) error {
	policy := os.Getenv(pullPolicyEnv)
	if policy == "" {
		policy = pullIfNotPresent
	}
	if policy == pullNever {
		return nil
	}
	_, err := backend.ImageInspect(ctx, image)
	if err == nil {
		if policy == pullIfNotPresent {
			return nil
		}
	}
	rc, err := backend.ImagePull(ctx, image, client.ImagePullOptions{})
	if err != nil {
		return fmt.Errorf("pull image %q: %w", image, err)
	}
	defer rc.Close()
	return rc.Wait(ctx)
}

// backendAdapter wraps *client.Client to implement DockerBackend (ExecAttach returns ExecAttachStream).
type backendAdapter struct {
	*client.Client
}

func (a *backendAdapter) ImagePull(ctx context.Context, ref string, opts client.ImagePullOptions) (imagePullResponse, error) {
	return a.Client.ImagePull(ctx, ref, opts)
}

func (a *backendAdapter) ExecAttach(ctx context.Context, execID string, opts client.ExecAttachOptions) (ExecAttachStream, error) {
	resp, err := a.Client.ExecAttach(ctx, execID, opts)
	if err != nil {
		return nil, err
	}
	return &attachStreamAdapter{ExecAttachResult: resp}, nil
}

// attachStreamAdapter adapts client.ExecAttachResult to ExecAttachStream (io.Reader + Close).
type attachStreamAdapter struct {
	client.ExecAttachResult
}

func (a *attachStreamAdapter) Read(p []byte) (n int, err error) {
	return a.Reader.Read(p)
}

func envMapToSlice(m map[string]string) []string {
	if len(m) == 0 {
		return nil
	}
	out := make([]string, 0, len(m))
	for k, v := range m {
		out = append(out, k+"="+v)
	}
	return out
}
