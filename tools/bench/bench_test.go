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

func TestReportBudgets(t *testing.T) {
	good := results{version: time.Millisecond, runs: time.Millisecond, overhead: time.Millisecond, peakRSS: bytesPerMB}
	cases := []struct {
		name  string
		tweak func(*results)
		want  string
	}{
		{name: "all within", tweak: func(*results) {}},
		{name: "at the limit", tweak: func(r *results) { r.version = versionLimit }},
		{name: "version", tweak: func(r *results) { r.version = versionLimit + 1 }, want: "hp --version"},
		{name: "runs", tweak: func(r *results) { r.runs = runsLimit + 1 }, want: "hp runs"},
		{name: "overhead", tweak: func(r *results) { r.overhead = overheadLimit + 1 }, want: "hp run to"},
		{name: "rss", tweak: func(r *results) { r.peakRSS = rssLimit + 1 }, want: "peak RSS"},
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
