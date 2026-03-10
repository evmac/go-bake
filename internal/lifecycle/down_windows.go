//go:build windows

package lifecycle

import (
	"fmt"
	"os"
)

// StopDaemon terminates the process identified by pid. On Windows uses Kill.
func StopDaemon(pid int) error {
	p, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("find process %d: %w", pid, err)
	}
	return p.Kill()
}
