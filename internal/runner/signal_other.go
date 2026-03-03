//go:build windows

package runner

import (
	"os"
	"os/exec"
)

func setProcessGroup(cmd *exec.Cmd) {}

func killProcessGroup(cmd *exec.Cmd, sig os.Signal) {
	if cmd.Process != nil {
		_ = cmd.Process.Signal(sig)
	}
}
