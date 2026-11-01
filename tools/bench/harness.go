package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
)

const (
	benchModel  = "ollama/bench"
	benchPrompt = "say hi"
	seedID      = "20260101-000000-%04x"
)

type harness struct {
	bin     string
	dir     string
	url     string
	lastRSS int64
}

func buildHP(out string) error {
	cmd := exec.Command("go", "build", "-trimpath", "-ldflags=-s -w", "-o", out, "./cmd/hp")
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	return cmd.Run()
}

func (h *harness) state(name string) string {
	return filepath.Join(h.dir, name)
}

func (h *harness) command(state string, args ...string) *exec.Cmd {
	cmd := exec.Command(h.bin, args...)
	cmd.Dir = h.dir
	cmd.Env = []string{"HOME=" + h.dir, "XDG_STATE_HOME=" + state, "PATH=" + os.Getenv("PATH")}
	return cmd
}

func (h *harness) runArgs() []string {
	return []string{"run", "-m", benchModel, "--base-url", h.url, benchPrompt}
}

func (h *harness) medianWall(state string, n int, args ...string) (time.Duration, error) {
	samples := make([]time.Duration, n)
	for i := range samples {
		cmd := h.command(state, args...)
		start := time.Now()
		if err := cmd.Run(); err != nil {
			return 0, fmt.Errorf("hp %s: %w", strings.Join(args, " "), err)
		}
		samples[i] = time.Since(start)
		h.lastRSS = max(h.lastRSS, peakRSS(cmd))
	}
	return median(samples), nil
}

func (h *harness) runOverhead(state string, n int) (time.Duration, int64, error) {
	samples := make([]time.Duration, n)
	var rss int64
	for i := range samples {
		got, used, err := h.timeFirstByte(state)
		if err != nil {
			return 0, 0, err
		}
		samples[i] = got
		rss = max(rss, used)
	}
	return median(samples), rss, nil
}

func (h *harness) timeFirstByte(state string) (time.Duration, int64, error) {
	cmd := h.command(state, h.runArgs()...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return 0, 0, err
	}
	start := time.Now()
	if err := cmd.Start(); err != nil {
		return 0, 0, err
	}
	var first [1]byte
	_, readErr := io.ReadFull(stdout, first[:])
	took := time.Since(start)
	_, _ = io.Copy(io.Discard, stdout)
	if err := errors.Join(readErr, cmd.Wait()); err != nil {
		return 0, 0, fmt.Errorf("hp run: %w", err)
	}
	return took, peakRSS(cmd), nil
}

func (h *harness) seedJournals(state string, n int) error {
	if err := h.command(state, h.runArgs()...).Run(); err != nil {
		return fmt.Errorf("seed run: %w", err)
	}
	runs := filepath.Join(state, "hp", "runs")
	matches, err := filepath.Glob(filepath.Join(runs, "*.jsonl"))
	if err != nil || len(matches) != 1 {
		return fmt.Errorf("seed journal: %d found, %v", len(matches), err)
	}
	return copyJournals(matches[0], runs, n)
}

func copyJournals(template, dir string, n int) error {
	data, err := os.ReadFile(template)
	if err != nil {
		return err
	}
	id := strings.TrimSuffix(filepath.Base(template), ".jsonl")
	for i := range n {
		next := fmt.Sprintf(seedID, i)
		body := bytes.ReplaceAll(data, []byte(id), []byte(next))
		if err := os.WriteFile(filepath.Join(dir, next+".jsonl"), body, 0o600); err != nil {
			return err
		}
	}
	return os.Remove(template)
}

func peakRSS(cmd *exec.Cmd) int64 {
	usage, ok := cmd.ProcessState.SysUsage().(*syscall.Rusage)
	if !ok {
		return 0
	}
	if runtime.GOOS == "darwin" {
		return usage.Maxrss
	}
	return usage.Maxrss * 1024
}
