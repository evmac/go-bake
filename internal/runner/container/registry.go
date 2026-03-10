package container

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"

	"github.com/evmac/go-bake/internal/config"
	"github.com/moby/moby/client"
)

// dockerNetVolClient is the minimal Docker API used by DockerNetVolRegistry (testable with a fake).
type dockerNetVolClient interface {
	NetworkCreate(ctx context.Context, name string, options client.NetworkCreateOptions) (client.NetworkCreateResult, error)
	VolumeCreate(ctx context.Context, options client.VolumeCreateOptions) (client.VolumeCreateResult, error)
}

// DockerNetVolRegistry is a NetVolRegistry that creates networks and volumes via the Docker API.
// First reference to a name creates the resource; later references return the same id/name.
// Safe for concurrent use.
type DockerNetVolRegistry struct {
	cli    dockerNetVolClient
	netMu  sync.Mutex
	netIDs map[string]string
	volMu  sync.Mutex
	volIDs map[string]string
}

// NewDockerNetVolRegistry returns a registry that uses the given Docker client (or a fake implementing dockerNetVolClient).
// Caller must ensure the client is closed when done when using a real *client.Client.
func NewDockerNetVolRegistry(cli dockerNetVolClient) *DockerNetVolRegistry {
	return &DockerNetVolRegistry{
		cli:    cli,
		netIDs: make(map[string]string),
		volIDs: make(map[string]string),
	}
}

// NewDockerRegistryFromEnv creates a Docker client from environment (DOCKER_HOST, etc.) and returns
// a NetVolRegistry and a close function. If the client cannot be created, returns an error.
// Call close() when the run is finished (e.g. defer close()).
func NewDockerRegistryFromEnv() (NetVolRegistry, func(), error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, nil, err
	}
	reg := NewDockerNetVolRegistry(cli)
	return reg, func() { cli.Close() }, nil
}

// EnsureNetwork creates the network if it does not exist and returns its ID (or name for connecting).
func (r *DockerNetVolRegistry) EnsureNetwork(ctx context.Context, name string) (string, error) {
	r.netMu.Lock()
	defer r.netMu.Unlock()
	if id, ok := r.netIDs[name]; ok {
		return id, nil
	}
	resp, err := r.cli.NetworkCreate(ctx, name, client.NetworkCreateOptions{})
	if err != nil {
		return "", err
	}
	r.netIDs[name] = resp.ID
	return resp.ID, nil
}

// EnsureVolume creates the volume if it does not exist (for named volumes) or resolves the host path (for bind mounts).
// Returns (hostSource, containerTarget, nil). For named volume: source is volume name, target is /mnt/<name>.
// For bind mount: source is absolute host path, target is /mnt/<name>.
func (r *DockerNetVolRegistry) EnsureVolume(ctx context.Context, ref config.VolumeRef, rootDir string) (hostSource, containerTarget string, err error) {
	containerTarget = "/mnt/" + ref.Name
	if ref.HostPath != "" {
		hostSource, err = filepath.Abs(filepath.Join(rootDir, ref.HostPath))
		if err != nil {
			return "", "", fmt.Errorf("resolve host path %q: %w", ref.HostPath, err)
		}
		return hostSource, containerTarget, nil
	}
	r.volMu.Lock()
	defer r.volMu.Unlock()
	if name, ok := r.volIDs[ref.Name]; ok {
		return name, containerTarget, nil
	}
	resp, err := r.cli.VolumeCreate(ctx, client.VolumeCreateOptions{Name: ref.Name})
	if err != nil {
		return "", "", err
	}
	r.volIDs[ref.Name] = resp.Volume.Name
	return resp.Volume.Name, containerTarget, nil
}
