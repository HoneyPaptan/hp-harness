package tool

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/HoneyPaptan/hp-harness/internal/host"
)

func TestGrep(t *testing.T) {
	ctx := context.Background()
	dir := "/w"
	base := host.NewMemory()
	must := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}
	must(base.WriteFile(dir+"/a.go", []byte("func main()\nother\n")))
	must(base.WriteFile(dir+"/a.txt", []byte("func main\n")))
	must(base.WriteFile(dir+"/sub/b.go", []byte("func main()\n")))
	must(base.WriteFile(dir+"/.git/config", []byte("func main\n")))
	must(base.WriteFile(dir+"/bin.dat", []byte("func main\x00\n")))
	tests := []struct {
		name    string
		h       *host.Memory
		raw     string
		wantErr bool
		check   func(*testing.T, Result)
	}{
		{
			name: "basic",
			h:    base,
			raw:  `{"pattern":"func main"}`,
			check: func(t *testing.T, r Result) {
				if strings.Count(r.Text, "func main") < 2 {
					t.Fatalf("%q", r.Text)
				}
			},
		},
		{
			name: "glob",
			h:    base,
			raw:  `{"pattern":"func main","glob":"*.go"}`,
			check: func(t *testing.T, r Result) {
				if strings.Contains(r.Text, ".txt") {
					t.Fatalf("%q", r.Text)
				}
			},
		},
		{
			name: "skip git",
			h:    base,
			raw:  `{"pattern":"func main"}`,
			check: func(t *testing.T, r Result) {
				if strings.Contains(r.Text, ".git") {
					t.Fatalf("%q", r.Text)
				}
			},
		},
		{
			name: "skip binary",
			h:    base,
			raw:  `{"pattern":"func main","path":"bin.dat"}`,
			check: func(t *testing.T, r Result) {
				if r.Text != "no matches" {
					t.Fatalf("%q", r.Text)
				}
			},
		},
		{
			name: "sort",
			h:    base,
			raw:  `{"pattern":"func main","glob":"*.go"}`,
			check: func(t *testing.T, r Result) {
				i := strings.Index(r.Text, "a.go:")
				j := strings.Index(r.Text, "sub/b.go:")
				if i < 0 || j < 0 || i > j {
					t.Fatalf("%q", r.Text)
				}
			},
		},
		{
			name: "single file",
			h:    base,
			raw:  `{"pattern":"other","path":"a.go"}`,
			check: func(t *testing.T, r Result) {
				if !strings.HasPrefix(r.Text, "a.go:2:") {
					t.Fatalf("%q", r.Text)
				}
			},
		},
		{
			name:    "invalid regexp",
			h:       base,
			raw:     `{"pattern":"("}`,
			wantErr: true,
		},
		{
			name:    "invalid glob",
			h:       base,
			raw:     `{"pattern":"x","glob":"["}`,
			wantErr: true,
		},
		{
			name: "no matches",
			h:    base,
			raw:  `{"pattern":"zzzunique"}`,
			check: func(t *testing.T, r Result) {
				if r.Text != "no matches" || r.IsError {
					t.Fatalf("%q", r.Text)
				}
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res, err := grepTool.Run(ctx, Env{Host: tc.h, Dir: dir}, json.RawMessage(tc.raw))
			if err != nil {
				t.Fatal(err)
			}
			if res.IsError != tc.wantErr {
				t.Fatalf("IsError=%v %q", res.IsError, res.Text)
			}
			if tc.check != nil {
				tc.check(t, res)
			}
		})
	}
}

func TestGrepCap(t *testing.T) {
	ctx := context.Background()
	dir := "/w"
	h := host.NewMemory()
	var b strings.Builder
	for i := 0; i < 250; i++ {
		b.WriteString("needle\n")
	}
	if err := h.WriteFile(dir+"/many.txt", []byte(b.String())); err != nil {
		t.Fatal(err)
	}
	res, err := grepTool.Run(ctx, Env{Host: h, Dir: dir}, json.RawMessage(`{"pattern":"needle"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(res.Text, "[capped at 200 matches]") {
		t.Fatalf("%q", res.Text)
	}
	if strings.Count(res.Text, "needle") != 200 {
		t.Fatalf("count %d", strings.Count(res.Text, "needle"))
	}
}
