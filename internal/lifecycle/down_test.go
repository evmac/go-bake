package lifecycle

import (
	"os/exec"
	"runtime"
	"testing"
	"time"
)

func TestStopDaemon(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("StopDaemon test uses sleep subprocess; run on Unix")
	}
	cmd := exec.Command("sleep", "10")
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		t.Skipf("cannot start sleep: %v", err)
	}
	pid := cmd.Process.Pid
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	if err := StopDaemon(pid); err != nil {
		t.Fatalf("StopDaemon: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
		// process exited after signal
	case <-time.After(2 * time.Second):
		t.Fatal("process did not exit after StopDaemon")
	}
}

func TestStopDaemon_invalidPid(t *testing.T) {
	// FindProcess never fails on Unix; signaling a non-existent process may return an error.
	// Use a high pid that is unlikely to exist (e.g. 1 is init and we can't signal it from user, or use 999999).
	err := StopDaemon(99999999)
	if err != nil {
		// Expected on most systems when process doesn't exist or we can't signal it
		return
	}
	// Some systems might not return an error for a non-existent pid
	t.Logf("StopDaemon(99999999) did not error (platform-dependent)")
}
