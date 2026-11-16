package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestMedian(t *testing.T) {
	cases := []struct {
		name    string
		samples []time.Duration
		want    time.Duration
	}{
		{name: "empty"},
		{name: "one", samples: []time.Duration{3}, want: 3},
		{name: "unsorted odd", samples: []time.Duration{9, 1, 5}, want: 5},
		{name: "unsorted even", samples: []time.Duration{4, 1, 3, 2}, want: 3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := median(tc.samples); got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func steady(d time.Duration) timed {
	return timed{wall: []time.Duration{d}, cpu: []time.Duration{d}}
}

func TestReportBudgets(t *testing.T) {
	good := results{version: steady(time.Millisecond), runs: steady(time.Millisecond), overhead: steady(time.Millisecond), peakRSS: bytesPerMB}
	cases := []struct {
		name  string
		tweak func(*results)
		want  string
	}{
		{name: "all within", tweak: func(*results) {}},
		{name: "at the limit", tweak: func(r *results) { r.version = steady(versionCPULimit) }},
		{name: "version cpu", tweak: func(r *results) { r.version.cpu = []time.Duration{versionCPULimit + 1} }, want: "hp --version"},
		{name: "version wall", tweak: func(r *results) { r.version.wall = []time.Duration{versionWallLimit + 1} }, want: "hp --version"},
		{name: "runs cpu", tweak: func(r *results) { r.runs.cpu = []time.Duration{runsCPULimit + 1} }, want: "hp runs"},
		{name: "runs wall", tweak: func(r *results) { r.runs.wall = []time.Duration{runsWallLimit + 1} }, want: "hp runs"},
		{name: "overhead cpu", tweak: func(r *results) { r.overhead.cpu = []time.Duration{overheadCPULimit + 1} }, want: "hp run to"},
		{name: "overhead wall", tweak: func(r *results) { r.overhead.wall = []time.Duration{overheadWallLimit + 1} }, want: "hp run to"},
		{name: "rss", tweak: func(r *results) { r.peakRSS = rssLimit + 1 }, want: "peak RSS"},
		{name: "slow wall under the ceiling passes", tweak: func(r *results) { r.version.wall = []time.Duration{versionWallLimit} }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := good
			tc.tweak(&r)
			var out bytes.Buffer
			err := report(&out, r)
			if tc.want == "" {
				if err != nil {
					t.Fatalf("err = %v\n%s", err, out.String())
				}
				return
			}
			if !errors.Is(err, errOverBudget) {
				t.Fatalf("err = %v", err)
			}
			for _, line := range strings.Split(out.String(), "\n") {
				if strings.HasSuffix(line, "OVER") && !strings.HasPrefix(line, tc.want) {
					t.Fatalf("wrong row: %q", line)
				}
			}
		})
	}
}

func TestPercentile(t *testing.T) {
	samples := []time.Duration{5, 1, 4, 2, 3, 9, 8, 7, 6, 10}
	if got := percentile(samples, 0.9); got != 9 {
		t.Fatalf("p90 = %v, want 9", got)
	}
	if got := percentile(nil, 0.9); got != 0 {
		t.Fatalf("empty = %v", got)
	}
}
