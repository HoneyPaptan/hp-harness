//go:build unix

package main

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

func spawnDetached(args []string, logPath string) (int, error) {
	exe, err := os.Executable()
	if err != nil {
		return 0, err
	}
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	cmd := exec.Command(exe, args...)
	cmd.Stdin, _ = os.Open(os.DevNull)
	cmd.Stdout = f
	cmd.Stderr = f
	cmd.Env = append(os.Environ(), "HP_DETACHED=1")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return 0, err
	}
	pid := cmd.Process.Pid
	if err := cmd.Process.Release(); err != nil {
		return 0, err
	}
	return pid, nil
}

func processAlive(pid int) bool {
	err := syscall.Kill(pid, 0)
	if err == nil {
		return true
	}
	return errors.Is(err, syscall.EPERM)
}

func processSignal(pid int) error {
	return syscall.Kill(pid, syscall.SIGINT)
}
