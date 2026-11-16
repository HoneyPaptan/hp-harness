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
	buildTags   = "nethttpomithttp2"
	seedID      = "20260101-000000-%04x"
)

type harness struct {
	bin     string
	dir     string
	url     string
	lastRSS int64
}

func buildHP(out string) error {
	cmd := exec.Command("go", "build", "-tags", buildTags, "-trimpath", "-ldflags=-s -w", "-o", out, "./cmd/hp")
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
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

type timed struct {
	wall, cpu []time.Duration
}

func (t *timed) add(wall time.Duration, cmd *exec.Cmd) {
	t.wall = append(t.wall, wall)
	t.cpu = append(t.cpu, cmd.ProcessState.UserTime()+cmd.ProcessState.SystemTime())
}

func (h *harness) wallSamples(state string, n int, args ...string) (timed, error) {
	var t timed
	for range n {
		cmd := h.command(state, args...)
		start := time.Now()
		if err := cmd.Run(); err != nil {
			return timed{}, fmt.Errorf("hp %s: %w", strings.Join(args, " "), err)
		}
		t.add(time.Since(start), cmd)
		h.lastRSS = max(h.lastRSS, peakRSS(cmd))
	}
	return t, nil
}

func (h *harness) firstByteSamples(state string, n int) (timed, error) {
	var t timed
	for range n {
		if err := h.timeFirstByte(state, &t); err != nil {
			return timed{}, err
		}
	}
	return t, nil
}

func (h *harness) timeFirstByte(state string, t *timed) error {
	cmd := h.command(state, h.runArgs()...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	start := time.Now()
	if err := cmd.Start(); err != nil {
		return err
	}
	var first [1]byte
	_, readErr := io.ReadFull(stdout, first[:])
	took := time.Since(start)
	_, _ = io.Copy(io.Discard, stdout)
	if err := errors.Join(readErr, cmd.Wait()); err != nil {
		return fmt.Errorf("hp run: %w", err)
	}
	t.add(took, cmd)
	h.lastRSS = max(h.lastRSS, peakRSS(cmd))
	return nil
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
