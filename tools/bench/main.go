package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"time"
)

const (
	versionSamples  = 50
	runsSamples     = 20
	overheadSamples = 30
	journalCount    = 500
	bytesPerMB      = 1 << 20
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "bench: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	dir, err := os.MkdirTemp("", "hp-bench")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	res, err := measureAll(dir)
	if err != nil {
		return err
	}
	return report(os.Stdout, res)
}

func measureAll(dir string) (results, error) {
	var res results
	bin := filepath.Join(dir, "hp")
	if err := buildHP(bin); err != nil {
		return res, err
	}
	srv, err := startFakeServer()
	if err != nil {
		return res, err
	}
	defer srv.Close()
	h := harness{bin: bin, dir: dir, url: srv.URL}
	if res.version, err = h.medianWall(h.state("empty"), versionSamples, "--version"); err != nil {
		return res, err
	}
	if res.overhead, res.peakRSS, err = h.runOverhead(h.state("run"), overheadSamples); err != nil {
		return res, err
	}
	state := h.state("runs")
	if err := h.seedJournals(state, journalCount); err != nil {
		return res, err
	}
	if res.runs, err = h.medianWall(state, runsSamples, "runs"); err != nil {
		return res, err
	}
	res.peakRSS = max(res.peakRSS, h.lastRSS)
	return res, nil
}

func median(samples []time.Duration) time.Duration {
	if len(samples) == 0 {
		return 0
	}
	sorted := slices.Sorted(slices.Values(samples))
	return sorted[len(sorted)/2]
}
