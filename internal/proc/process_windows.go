//go:build windows

package proc

import (
	"fmt"
	"os"
)

func IsRunning(pid int) bool {
	// Windows: use os.FindProcess — if it succeeds the process slot exists.
	if pid <= 0 {
		return false
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// On Windows FindProcess always succeeds; there is no signal 0. We treat
	// any found process as running (gateway is typically Linux/Mac anyway).
	_ = p
	return true
}

func StopByPIDFile(pidFile string) error {
	pid, err := ReadPID(pidFile)
	if err != nil {
		return err
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("process %d not found: %w", pid, err)
	}
	if err := p.Kill(); err != nil {
		return err
	}
	_ = removeFile(pidFile)
	return nil
}
