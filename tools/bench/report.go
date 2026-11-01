package main

import (
	"errors"
	"fmt"
	"io"
	"time"
)

const (
	versionLimit  = 10 * time.Millisecond
	runsLimit     = 100 * time.Millisecond
	overheadLimit = 15 * time.Millisecond
	rssLimit      = 40 * bytesPerMB
)

var errOverBudget = errors.New("over budget")

type results struct {
	version  time.Duration
	runs     time.Duration
	overhead time.Duration
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
		timeRow("hp --version", r.version, versionLimit),
		timeRow("hp runs, 500 journals", r.runs, runsLimit),
		timeRow("hp run to first byte", r.overhead, overheadLimit),
		{
			name: "peak RSS", got: fmt.Sprintf("%.1f MB", float64(r.peakRSS)/bytesPerMB),
			limit: fmt.Sprintf("%d MB", rssLimit/bytesPerMB), failed: r.peakRSS > rssLimit,
		},
	}
}

func timeRow(name string, got, limit time.Duration) row {
	return row{name: name, got: got.Round(10 * time.Microsecond).String(), limit: limit.String(), failed: got > limit}
}

func report(w io.Writer, r results) error {
	var failed bool
	for _, line := range r.rows() {
		fmt.Fprintf(w, "%-24s %10s  limit %-6s %s\n", line.name, line.got, line.limit, verdict(line.failed))
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
