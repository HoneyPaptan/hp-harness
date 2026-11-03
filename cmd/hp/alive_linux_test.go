//go:build linux

package main

import (
	"os"
	"os/exec"
	"syscall"
	"testing"
)

func TestProcessAliveSeesSelf(t *testing.T) {
	if !processAlive(os.Getpid()) {
		t.Fatal("our own pid counted dead")
	}
}

func TestProcessAliveForeignImage(t *testing.T) {
	cmd := exec.Command("/bin/sleep", "30")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Pgid: 0}
	if err := cmd.Start(); err != nil {
		t.Skip("cannot start /bin/sleep")
	}
	defer func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}()
	pid := cmd.Process.Pid
	if processAlive(pid) {
		t.Fatalf("pid %d runs %q, not hp", pid, cmd.Path)
	}
}

func TestProcessAliveGone(t *testing.T) {
	cmd := exec.Command("/bin/sleep", "0.01")
	if err := cmd.Start(); err != nil {
		t.Skip("cannot start /bin/sleep")
	}
	pid := cmd.Process.Pid
	_ = cmd.Wait()
	if processAlive(pid) {
		t.Fatalf("pid %d should be gone", pid)
	}
}

func TestProcessStartStable(t *testing.T) {
	a := processStart(os.Getpid())
	if a == "" || a != processStart(os.Getpid()) {
		t.Fatalf("start %q", a)
	}
	if processStart(1<<30) != "" {
		t.Fatal("start for missing pid")
	}
}
