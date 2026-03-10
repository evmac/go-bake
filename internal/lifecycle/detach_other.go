//go:build windows

package lifecycle

import "os/exec"

func setDetached(cmd *exec.Cmd) {
	// On Windows there is no Setsid; process may not survive parent exit.
	_ = cmd
}
