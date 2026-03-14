package baked

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/evmac/go-bake/internal/lifecycle"
)

func TestIsDaemonAliveDeadPID(t *testing.T) {
	alive := isDaemonAlive(lifecycle.DaemonEntry{PID: 99999999})
	if alive {
		t.Error("expected false for non-existent PID")
	}
}

func TestIsDaemonAliveZeroPID(t *testing.T) {
	alive := isDaemonAlive(lifecycle.DaemonEntry{PID: 0})
	if alive {
		t.Error("expected false for PID 0")
	}
}

func TestIsDaemonAliveCurrentProcess(t *testing.T) {
	alive := isDaemonAlive(lifecycle.DaemonEntry{PID: os.Getpid()})
	if !alive {
		t.Error("expected true for current process PID")
	}
}

func TestIsDaemonAliveFakeContainer(t *testing.T) {
	alive := isDaemonAlive(lifecycle.DaemonEntry{ContainerID: "deadbeef123456"})
	// Docker not necessarily running in CI; expect false
	if alive {
		t.Log("container reported alive — docker might be running")
	}
}

func TestIsContainerAliveNoDocker(t *testing.T) {
	alive := isContainerAlive("nonexistent_container_id")
	if alive {
		t.Error("expected false for nonexistent container")
	}
}

func TestHealthCheckerStartStop(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".bake"), 0755)
	hc := StartHealthChecker(dir)
	hc.Stop()
}

func TestHealthCheckerCheckCleansStale(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".bake"), 0755)

	// Add a fake daemon with a dead PID
	if err := lifecycle.AddDaemon(dir, "dead-daemon", 99999999, ""); err != nil {
		t.Fatal(err)
	}

	hc := &HealthChecker{rootDir: dir}
	hc.check()

	// Verify it was cleaned
	state, err := lifecycle.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := state.Daemons["dead-daemon"]; ok {
		t.Error("expected dead daemon to be removed from state")
	}
}

func TestStatusDump(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".bake"), 0755)
	status := StatusDump(dir, nil)
	if status == "" {
		t.Error("expected non-empty status dump")
	}
	if len(status) < 10 {
		t.Errorf("status too short: %q", status)
	}
}

func TestStatusDumpWithDaemon(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".bake"), 0755)

	lifecycle.AddDaemon(dir, "myservice", os.Getpid(), "")

	status := StatusDump(dir, nil)
	if status == "" {
		t.Error("expected non-empty status dump")
	}
}

func TestStatusDumpWithContainer(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".bake"), 0755)

	lifecycle.AddDaemon(dir, "db", 0, "abc123def456789")

	status := StatusDump(dir, nil)
	if status == "" {
		t.Error("expected non-empty status dump")
	}
}

func TestMinFunction(t *testing.T) {
	if min(3, 5) != 3 {
		t.Error("min(3,5) should be 3")
	}
	if min(5, 3) != 3 {
		t.Error("min(5,3) should be 3")
	}
	if min(4, 4) != 4 {
		t.Error("min(4,4) should be 4")
	}
}
