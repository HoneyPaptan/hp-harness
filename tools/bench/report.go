package main

import (
	"errors"
	"fmt"
	"io"
	"time"
)

const (
	versionCPULimit   = 8 * time.Millisecond
	runsCPULimit      = 14 * time.Millisecond
	overheadCPULimit  = 14 * time.Millisecond
	versionWallLimit  = 50 * time.Millisecond
	runsWallLimit     = 100 * time.Millisecond
	overheadWallLimit = 100 * time.Millisecond
	rssLimit          = 20 * bytesPerMB
)

var errOverBudget = errors.New("over budget")

type results struct {
	version  timed
	runs     timed
	overhead timed
	peakRSS  int64
}

type row struct {
	name   string
	got    string
	limit  string
	failed bool
}

func (r results) rows() []row {
	return []row{
		timeRows("hp --version", r.version, versionCPULimit, versionWallLimit),
		timeRows("hp runs, 500 journals", r.runs, runsCPULimit, runsWallLimit),
		timeRows("hp run to first byte", r.overhead, overheadCPULimit, overheadWallLimit),
		{
			name: "peak RSS", got: fmt.Sprintf("%.1f MB", float64(r.peakRSS)/bytesPerMB),
			limit: fmt.Sprintf("%d MB", rssLimit/bytesPerMB), failed: r.peakRSS > rssLimit,
		},
	}
}

func timeRows(name string, t timed, cpuLimit, wallLimit time.Duration) row {
	cpu, wall := median(t.cpu), median(t.wall)
	return row{
		name:   name,
		got:    fmt.Sprintf("cpu %s  wall %s p90 %s", round(cpu), round(wall), round(percentile(t.wall, 0.9))),
		limit:  fmt.Sprintf("cpu %s  wall %s", cpuLimit, wallLimit),
		failed: cpu > cpuLimit || wall > wallLimit,
	}
}

func round(d time.Duration) time.Duration {
	return d.Round(10 * time.Microsecond)
}

func report(w io.Writer, r results) error {
	var failed bool
	for _, line := range r.rows() {
		fmt.Fprintf(w, "%-22s %-38s limit %-22s %s\n", line.name, line.got, line.limit, verdict(line.failed))
		failed = failed || line.failed
	}
	if failed {
		return errOverBudget
	}
	return nil
}

func verdict(failed bool) string {
	if failed {
		return "OVER"
	}
	return "ok"
}
