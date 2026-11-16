package cli

import (
	"io"
	"testing"

	"github.com/HoneyPaptan/hp-harness/internal/host"
)

func BenchmarkRunLines(b *testing.B) {
	env := Env{Host: host.Native{}, Getenv: func(k string) string {
		if k == "XDG_STATE_HOME" {
			return "/tmp/perf500/s"
		}
		return ""
	}, Stderr: io.Discard, Alive: func(int) bool { return false }}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := runLines(env); err != nil {
			b.Fatal(err)
		}
	}
}
