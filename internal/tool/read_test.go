package tool

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/HoneyPaptan/hp-harness/internal/host"
)

func TestRead(t *testing.T) {
	ctx := context.Background()
	dir := "/w"
	mk := func(body string) *host.Memory {
		h := host.NewMemory()
		if err := h.WriteFile(dir+"/f.txt", []byte(body)); err != nil {
			t.Fatal(err)
		}
		return h
	}
	ten := strings.Join([]string{"l1", "l2", "l3", "l4", "l5", "l6", "l7", "l8", "l9", "l10"}, "\n") + "\n"
	tests := []struct {
		name        string
		h           *host.Memory
		raw         string
		wantErr     bool
		contains    []string
		notContains []string
	}{
		{
			name:        "offset limit",
			h:           mk(ten),
			raw:         `{"path":"f.txt","offset":3,"limit":2}`,
			contains:    []string{"     3\tl3", "     4\tl4"},
			notContains: []string{"     2\t", "     5\t"},
		},
		{
			name:     "defaults",
			h:        mk("only\n"),
			raw:      `{"path":"f.txt"}`,
			contains: []string{"     1\tonly"},
		},
		{
			name:     "missing",
			h:        host.NewMemory(),
			raw:      `{"path":"nope.txt"}`,
			wantErr:  true,
			contains: []string{"file not found: nope.txt"},
		},
		{
			name: "directory",
			h: func() *host.Memory {
				h := host.NewMemory()
				if err := h.WriteFile(dir+"/d/x", []byte("x")); err != nil {
					t.Fatal(err)
				}
				return h
			}(),
			raw:      `{"path":"d"}`,
			wantErr:  true,
			contains: []string{"path is a directory: d"},
		},
		{
			name:     "offset past end",
			h:        mk("one\n"),
			raw:      `{"path":"f.txt","offset":5}`,
			wantErr:  true,
			contains: []string{"offset past end of file: f.txt"},
		},
		{
			name:    "unknown field",
			h:       mk("x\n"),
			raw:     `{"path":"f.txt","extra":1}`,
			wantErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res, err := readTool.Run(ctx, Env{Host: tc.h, Dir: dir}, json.RawMessage(tc.raw))
			if err != nil {
				t.Fatal(err)
			}
			if tc.wantErr != res.IsError {
				t.Fatalf("IsError=%v text=%q", res.IsError, res.Text)
			}
			for _, s := range tc.contains {
				if !strings.Contains(res.Text, s) {
					t.Fatalf("missing %q in %q", s, res.Text)
				}
			}
			for _, s := range tc.notContains {
				if strings.Contains(res.Text, s) {
					t.Fatalf("unexpected %q in %q", s, res.Text)
				}
			}
		})
	}
}
