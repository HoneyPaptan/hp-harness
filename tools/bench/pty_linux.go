//go:build linux

package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"syscall"
	"time"
	"unsafe"
)

const (
	tiocgptn   = 0x80045430
	tiocsptlck = 0x40045431
	tiocswinsz = 0x5414
	promptWait = 5 * time.Second
)

type winsize struct{ rows, cols, x, y uint16 }

func openPty() (*os.File, *os.File, error) {
	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR, 0)
	if err != nil {
		return nil, nil, err
	}
	var unlock, n uint32
	size := winsize{rows: 40, cols: 120}
	for _, call := range []struct {
		req uintptr
		arg unsafe.Pointer
	}{{tiocsptlck, unsafe.Pointer(&unlock)}, {tiocgptn, unsafe.Pointer(&n)}, {tiocswinsz, unsafe.Pointer(&size)}} {
		if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, master.Fd(), call.req, uintptr(call.arg)); errno != 0 {
			master.Close()
			return nil, nil, errno
		}
	}
	slave, err := os.OpenFile(fmt.Sprintf("/dev/pts/%d", n), os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		master.Close()
		return nil, nil, err
	}
	return master, slave, nil
}

func (h *harness) promptSamples(state string, n int) ([]time.Duration, error) {
	samples := make([]time.Duration, n)
	for i := range samples {
		took, err := h.timePrompt(state)
		if err != nil {
			return nil, err
		}
		samples[i] = took
	}
	return samples, nil
}

func (h *harness) timePrompt(state string) (time.Duration, error) {
	master, slave, err := openPty()
	if err != nil {
		return 0, err
	}
	defer master.Close()
	cmd := h.command(state, "-m", benchModel, "--base-url", h.url)
	cmd.Env = append(cmd.Env, "TERM=xterm-256color")
	cmd.Stdin, cmd.Stdout, cmd.Stderr = slave, slave, slave
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true}
	start := time.Now()
	if err := cmd.Start(); err != nil {
		slave.Close()
		return 0, err
	}
	slave.Close()
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()
	return waitForPrompt(master, start)
}

func waitForPrompt(master *os.File, start time.Time) (time.Duration, error) {
	deadline := start.Add(promptWait)
	var seen []byte
	buf := make([]byte, 4096)
	for time.Now().Before(deadline) {
		_ = master.SetReadDeadline(deadline)
		got, err := master.Read(buf)
		seen = append(seen, buf[:got]...)
		if bytes.Contains(seen, []byte(benchModel)) {
			return time.Since(start), nil
		}
		if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, os.ErrDeadlineExceeded) {
			return 0, err
		}
	}
	return 0, fmt.Errorf("no prompt within %s, got %q", promptWait, seen)
}
