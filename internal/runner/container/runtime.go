package container

import (
	"os"
	"path/filepath"

	"github.com/moby/moby/client"
)

const (
	envRuntime        = "BAKE_RUNTIME"
	runtimeDocker     = "docker"
	runtimePodman     = "podman"
	runtimeContainerd = "containerd"
	runtimeCrio       = "crio"
	envDockerHost     = "DOCKER_HOST"
)

// DefaultPodmanSocket returns the default Podman API socket when DOCKER_HOST is not set.
// Rootless: $XDG_RUNTIME_DIR/podman/podman.sock; root: /run/podman/podman.sock.
func DefaultPodmanSocket() string {
	if dir := os.Getenv("XDG_RUNTIME_DIR"); dir != "" {
		return "unix://" + filepath.Join(dir, "podman", "podman.sock")
	}
	return "unix:///run/podman/podman.sock"
}

// ClientOpts returns Docker/OCI client options based on BAKE_RUNTIME and DOCKER_HOST.
// Supported runtimes: docker (default), podman, containerd, crio.
//   - docker: use FromEnv (DOCKER_HOST, etc.).
//   - podman: if DOCKER_HOST is set, use it; else use default Podman socket.
//   - containerd / crio: use FromEnv; user must set DOCKER_HOST to a Docker-compatible
//     endpoint (e.g. nerdctl or cri-dockerd).
func ClientOpts() []client.Opt {
	runtime := os.Getenv(envRuntime)
	if runtime == "" {
		runtime = runtimeDocker
	}
	switch runtime {
	case runtimePodman:
		if host := os.Getenv(envDockerHost); host != "" {
			return []client.Opt{client.FromEnv, client.WithAPIVersionNegotiation()}
		}
		return []client.Opt{client.WithHost(DefaultPodmanSocket()), client.WithAPIVersionNegotiation()}
	case runtimeContainerd, runtimeCrio:
		// User must set DOCKER_HOST to a Docker-compatible socket (e.g. nerdctl).
		fallthrough
	case runtimeDocker:
	default:
		// Unknown runtime: treat as docker (FromEnv).
	}
	return []client.Opt{client.FromEnv, client.WithAPIVersionNegotiation()}
}
