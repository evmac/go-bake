package lifecycle

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/evmac/go-bake/internal/config"
	"github.com/evmac/go-bake/internal/runner/container"
)

func TestStartDaemon_noSteps(t *testing.T) {
	d := &config.Daemon{Name: "empty"}
	_, _, err := StartDaemon(context.Background(), t.TempDir(), d, nil, nil, nil, nil)
	if err == nil {
		t.Fatal("expected error for daemon with no steps")
	}
	if err.Error() != `daemon "empty" has no steps` {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestStartDaemon_emptyArgv(t *testing.T) {
	d := &config.Daemon{Name: "bad", Steps: []config.Step{{Argv: nil}}}
	_, _, err := StartDaemon(context.Background(), t.TempDir(), d, nil, nil, nil, nil)
	if err == nil {
		t.Fatal("expected error for empty argv")
	}
	if err.Error() != `daemon "bad" step has empty argv` {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestStartDaemon_success(t *testing.T) {
	dir := t.TempDir()
	// Use a short sleep so the process exits quickly; we only need to verify StartDaemon returns a PID.
	d := &config.Daemon{
		Name:  "sleeper",
		Steps: []config.Step{{Argv: []string{"sleep", "0.01"}}},
	}
	pid, containerID, err := StartDaemon(context.Background(), dir, d, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("StartDaemon: %v", err)
	}
	if pid <= 0 {
		t.Errorf("expected positive PID, got %d", pid)
	}
	if containerID != "" {
		t.Errorf("expected host daemon, got containerID %q", containerID)
	}
	// Give the child a moment to exit so we don't leave zombies
	time.Sleep(50 * time.Millisecond)
}

func TestStartDaemon_withEnvOverlay(t *testing.T) {
	dir := t.TempDir()
	d := &config.Daemon{
		Name:  "envtest",
		Steps: []config.Step{{Argv: []string{"sleep", "0.01"}}},
		Env:   map[string]string{"DAEMON_FOO": "from-daemon"},
	}
	overlay := map[string]string{"DAEMON_FOO": "overridden", "PROFILE_BAR": "from-profile"}
	pid, _, err := StartDaemon(context.Background(), dir, d, nil, nil, overlay, nil)
	if err != nil {
		t.Fatalf("StartDaemon: %v", err)
	}
	if pid <= 0 {
		t.Errorf("expected positive PID, got %d", pid)
	}
	time.Sleep(50 * time.Millisecond)
}

func TestStartDaemon_withCwd(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatal(err)
	}
	d := &config.Daemon{
		Name:  "cwdtest",
		Cwd:   "sub",
		Steps: []config.Step{{Argv: []string{"sleep", "0.01"}}},
	}
	pid, _, err := StartDaemon(context.Background(), dir, d, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("StartDaemon: %v", err)
	}
	if pid <= 0 {
		t.Errorf("expected positive PID, got %d", pid)
	}
	time.Sleep(50 * time.Millisecond)
}

func TestStartDaemon_shellStep(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell step test uses sleep; adjust for Windows if needed")
	}
	dir := t.TempDir()
	d := &config.Daemon{
		Name:  "shelltest",
		Steps: []config.Step{{Runner: "sh", Argv: []string{"-c", "sleep 0.01"}}},
	}
	pid, _, err := StartDaemon(context.Background(), dir, d, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("StartDaemon: %v", err)
	}
	if pid <= 0 {
		t.Errorf("expected positive PID, got %d", pid)
	}
	time.Sleep(50 * time.Millisecond)
}

func TestStartDaemon_withImage_usesContainer(t *testing.T) {
	dir := t.TempDir()
	d := &config.Daemon{
		Name:   "redis",
		Image:  "redis:7",
		Steps:  []config.Step{{Argv: []string{"redis-server"}}},
	}
	opts := &StartDaemonOpts{
		Backend:  container.NewFakeBackend(),
		Registry: container.NewFakeNetVolRegistry(),
	}
	pid, containerID, err := StartDaemon(context.Background(), dir, d, nil, nil, nil, opts)
	if err != nil {
		t.Fatalf("StartDaemon with image: %v", err)
	}
	if pid != 0 {
		t.Errorf("expected pid 0 for container daemon, got %d", pid)
	}
	if containerID == "" {
		t.Error("expected containerID for daemon with image")
	}
}
