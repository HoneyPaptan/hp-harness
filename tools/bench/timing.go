package main

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"slices"
	"text/tabwriter"
	"time"

	"github.com/HoneyPaptan/hp-harness/internal/host"
	"github.com/HoneyPaptan/hp-harness/internal/journal"
	"github.com/HoneyPaptan/hp-harness/internal/provider"
)

const timingLoadLimit = 2.0

type timingRow struct {
	name string
	timed
}

func runTiming(n int, bin string) error {
	if n < 1 {
		return fmt.Errorf("-n must be at least 1")
	}
	warnLoad(os.Stderr)
	dir, err := os.MkdirTemp("", "hp-timing")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	rows, err := timeAll(dir, bin, n)
	if err != nil {
		return err
	}
	return printTiming(os.Stdout, rows, n)
}

func warnLoad(w io.Writer) {
	data, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return
	}
	var load float64
	if _, err := fmt.Sscanf(string(data), "%f", &load); err == nil && load > timingLoadLimit {
		fmt.Fprintf(w, "note: load average %.2f is above %.0f, numbers will be noisy\n", load, timingLoadLimit)
	}
}

func timeAll(dir, bin string, n int) ([]timingRow, error) {
	if bin == "" {
		bin = filepath.Join(dir, "hp")
		if err := buildHP(bin); err != nil {
			return nil, err
		}
	}
	srv, err := startFakeServer()
	if err != nil {
		return nil, err
	}
	defer srv.Close()
	h := harness{bin: bin, dir: dir, url: srv.URL}
	version, err := h.wallSamples(h.state("empty"), n, "--version")
	if err != nil {
		return nil, err
	}
	state := h.state("runs")
	if err := h.seedJournals(state, journalCount); err != nil {
		return nil, err
	}
	runs, err := h.wallSamples(state, n, "runs")
	if err != nil {
		return nil, err
	}
	prompt, err := h.promptSamples(h.state("prompt"), n)
	if err != nil {
		return nil, err
	}
	first, err := h.firstByteSamples(h.state("run"), n)
	if err != nil {
		return nil, err
	}
	turn, err := appendSamples(filepath.Join(dir, "append"), n)
	if err != nil {
		return nil, err
	}
	return []timingRow{
		{"hp --version", version},
		{"hp runs, 500 journals", runs},
		{"hp to first drawn prompt (pty)", prompt},
		{"hp run to first byte", first},
		{"journal append, one turn (4 records)", timed{wall: turn}},
	}, nil
}

func appendSamples(dir string, n int) ([]time.Duration, error) {
	w, err := journal.Create(host.Native{}, filepath.Join(dir, "turn.jsonl"), journal.RunStart{RunID: "timing"})
	if err != nil {
		return nil, err
	}
	samples := make([]time.Duration, n)
	for i := range samples {
		start := time.Now()
		if err := appendTurn(w); err != nil {
			return nil, err
		}
		samples[i] = time.Since(start)
	}
	return samples, nil
}

func appendTurn(w *journal.Writer) error {
	call := provider.ToolCall{ID: "c1", Name: "bash", Args: json.RawMessage("{\"command\":\"ls\"}")}
	reply := provider.Message{Role: provider.RoleAssistant, Text: "listing", ToolCalls: []provider.ToolCall{call}}
	steps := []error{
		w.User("list the files"),
		w.Assistant(reply),
		w.ToolStarted(call),
		w.ToolResult(provider.ToolResult{CallID: "c1", Text: "a.go\nb.go"}),
	}
	for _, err := range steps {
		if err != nil {
			return err
		}
	}
	return nil
}

func percentile(samples []time.Duration, p float64) time.Duration {
	if len(samples) == 0 {
		return 0
	}
	sorted := slices.Sorted(slices.Values(samples))
	return sorted[min(len(sorted)-1, int(math.Ceil(p*float64(len(sorted))))-1)]
}

func printTiming(w io.Writer, rows []timingRow, n int) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "path (n=%d)\tmedian\tp90\tcpu median\n", n)
	for _, r := range rows {
		cpu := "-"
		if len(r.cpu) > 0 {
			cpu = roundTiming(median(r.cpu)).String()
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", r.name, roundTiming(median(r.wall)), roundTiming(percentile(r.wall, 0.9)), cpu)
	}
	return tw.Flush()
}

func roundTiming(d time.Duration) time.Duration {
	return d.Round(10 * time.Microsecond)
}
