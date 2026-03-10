//go:build !windows

package lifecycle

import (
	"fmt"
	"os"
	"syscall"
)

// StopDaemon sends SIGTERM to the process identified by pid. It does not remove the entry from state;
// the caller should call RemoveDaemon after a successful stop.
func StopDaemon(pid int) error {
	p, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("find process %d: %w", pid, err)
	}
	return p.Signal(syscall.SIGTERM)
}
