package baked

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/evmac/go-bake/internal/lifecycle"
)

const (
	healthInterval = 30 * time.Second
	sleepThreshold = 3 * healthInterval // if a tick takes 3x longer, we probably slept
)

// HealthChecker periodically validates daemon PIDs/containers and cleans stale entries.
type HealthChecker struct {
	rootDir string
	cancel  context.CancelFunc
	done    chan struct{}
}

// StartHealthChecker begins periodic health checks. Call Stop() to end.
func StartHealthChecker(rootDir string) *HealthChecker {
	ctx, cancel := context.WithCancel(context.Background())
	hc := &HealthChecker{rootDir: rootDir, cancel: cancel, done: make(chan struct{})}
	go hc.run(ctx)
	return hc
}

// Stop stops the health checker.
func (hc *HealthChecker) Stop() {
	hc.cancel()
	<-hc.done
}

func (hc *HealthChecker) run(ctx context.Context) {
	defer close(hc.done)
	ticker := time.NewTicker(healthInterval)
	defer ticker.Stop()
	lastTick := time.Now()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			elapsed := now.Sub(lastTick)
			if elapsed > sleepThreshold {
				log.Printf("health: detected long sleep (%s), running full check", elapsed.Round(time.Second))
			}
			lastTick = now
			hc.check()
		}
	}
}

// check validates all daemon entries in state and removes dead ones.
func (hc *HealthChecker) check() {
	state, err := lifecycle.Load(hc.rootDir)
	if err != nil {
		return
	}
	for name, entry := range state.Daemons {
		alive := isDaemonAlive(entry)
		if !alive {
			log.Printf("health: daemon %q is dead (pid=%d container=%q), removing from state", name, entry.PID, entry.ContainerID)
			_ = lifecycle.RemoveDaemon(hc.rootDir, name)
		}
	}
}

// isDaemonAlive checks if a daemon is still running.
func isDaemonAlive(entry lifecycle.DaemonEntry) bool {
	if entry.ContainerID != "" {
		return isContainerAlive(entry.ContainerID)
	}
	if entry.PID <= 0 {
		return false
	}
	proc, err := os.FindProcess(entry.PID)
	if err != nil {
		return false
	}
	// On Unix, signal 0 checks if the process exists without sending a real signal.
	err = proc.Signal(syscall.Signal(0))
	return err == nil
}

// isContainerAlive checks if a container is running via docker inspect.
func isContainerAlive(containerID string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := runCmd(ctx, "docker", "inspect", "-f", "{{.State.Running}}", containerID)
	if err != nil {
		return false
	}
	return strings.TrimSpace(out) == "true"
}

func runCmd(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	b, err := cmd.Output()
	return string(b), err
}

// StatusDump returns a human-readable status string for the given workspace.
func StatusDump(rootDir string, _ interface{}) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "baked status\n")
	fmt.Fprintf(&sb, "  workspace: %s\n", rootDir)
	fmt.Fprintf(&sb, "  pid: %d\n", os.Getpid())
	state, err := lifecycle.Load(rootDir)
	if err != nil {
		fmt.Fprintf(&sb, "  state: error (%v)\n", err)
	} else {
		fmt.Fprintf(&sb, "  daemons: %d\n", len(state.Daemons))
		for name, entry := range state.Daemons {
			alive := isDaemonAlive(entry)
			status := "alive"
			if !alive {
				status = "dead"
			}
			if entry.ContainerID != "" {
				fmt.Fprintf(&sb, "    %s: container=%s (%s)\n", name, entry.ContainerID[:min(12, len(entry.ContainerID))], status)
			} else {
				fmt.Fprintf(&sb, "    %s: pid=%d (%s)\n", name, entry.PID, status)
			}
		}
	}
	return sb.String()
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
