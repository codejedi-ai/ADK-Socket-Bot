//go:build !windows

package proc

import "syscall"

func IsRunning(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil
}

func StopByPIDFile(pidFile string) error {
	pid, err := ReadPID(pidFile)
	if err != nil {
		return err
	}
	if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
		return err
	}
	_ = removeFile(pidFile)
	return nil
}
