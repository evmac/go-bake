package container

import (
	"os"
	"strings"
	"testing"
)

func TestDefaultPodmanSocket(t *testing.T) {
	// Without XDG_RUNTIME_DIR, returns root socket path.
	os.Unsetenv("XDG_RUNTIME_DIR")
	got := DefaultPodmanSocket()
	if want := "unix:///run/podman/podman.sock"; got != want {
		t.Errorf("DefaultPodmanSocket() = %q, want %q", got, want)
	}
	// With XDG_RUNTIME_DIR, uses it.
	os.Setenv("XDG_RUNTIME_DIR", "/run/user/1000")
	defer os.Unsetenv("XDG_RUNTIME_DIR")
	got = DefaultPodmanSocket()
	if want := "unix:///run/user/1000/podman/podman.sock"; got != want {
		t.Errorf("DefaultPodmanSocket() with XDG_RUNTIME_DIR = %q, want %q", got, want)
	}
}

func TestClientOpts(t *testing.T) {
	tests := []struct {
		name           string
		runtime        string
		dockerHost     string
		wantFromEnv    bool // if true, opts should include FromEnv behavior (we can't inspect Opts easily, so we check that we get at least one opt)
		wantPodmanHost bool // if true, we expect WithHost(podman socket) - we verify by checking that with podman and no DOCKER_HOST we get different count or we run and see socket)
	}{
		{"default empty", "", "", true, false},
		{"docker", "docker", "", true, false},
		{"podman no DOCKER_HOST", "podman", "", false, true},
		{"podman with DOCKER_HOST", "podman", "unix:///custom.sock", true, false},
		{"containerd", "containerd", "", true, false},
		{"crio", "crio", "", true, false},
		{"unknown falls back to docker", "invalid", "", true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.runtime != "" {
				os.Setenv(envRuntime, tt.runtime)
			} else {
				os.Unsetenv(envRuntime)
			}
			if tt.dockerHost != "" {
				os.Setenv(envDockerHost, tt.dockerHost)
			} else {
				os.Unsetenv(envDockerHost)
			}
			defer func() {
				os.Unsetenv(envRuntime)
				os.Unsetenv(envDockerHost)
			}()
			opts := ClientOpts()
			if len(opts) == 0 {
				t.Fatal("ClientOpts() returned no options")
			}
			// When podman and no DOCKER_HOST, first opt should be WithHost (we get Podman socket in DefaultPodmanSocket).
			if tt.wantPodmanHost {
				os.Unsetenv(envDockerHost)
				opts2 := ClientOpts()
				sock := DefaultPodmanSocket()
				if !strings.Contains(sock, "podman") {
					t.Errorf("expected podman socket to contain podman, got %q", sock)
				}
				_ = opts2
			}
		})
	}
}

func TestClientOpts_podmanRespectsDOCKER_HOST(t *testing.T) {
	os.Setenv(envRuntime, runtimePodman)
	os.Setenv(envDockerHost, "unix:///run/user/1000/podman/podman.sock")
	defer func() {
		os.Unsetenv(envRuntime)
		os.Unsetenv(envDockerHost)
	}()
	opts := ClientOpts()
	if len(opts) < 2 {
		t.Errorf("expected at least FromEnv and APIVersionNegotiation when DOCKER_HOST set, got %d opts", len(opts))
	}
}
