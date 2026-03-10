package lifecycle

import (
	"context"

	"github.com/evmac/go-bake/internal/runner/container"
)

// StopDaemonContainer stops and removes a container daemon (started with StartDaemon when daemon had image).
// Uses Docker from environment (DOCKER_HOST, BAKE_RUNTIME). Safe to call if the container is already removed.
func StopDaemonContainer(ctx context.Context, containerID string) error {
	return container.StopContainer(ctx, containerID, nil)
}
