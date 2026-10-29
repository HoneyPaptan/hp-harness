package tool

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/HoneyPaptan/hp-harness/internal/host"
)

func TestWrite(t *testing.T) {
	ctx := context.Background()
	dir := "/w"
	tests := []struct {
		name    string
		setup   func(*host.Memory)
		raw     string
		wantErr bool
		check   func(*testing.T, *host.Memory)
	}{
		{
			name: "nested new",
			raw:  `{"path":"a/b/c.txt","content":"hello"}`,
			check: func(t *testing.T, h *host.Memory) {
				data, err := h.ReadFile(dir + "/a/b/c.txt")
				if err != nil {
					t.Fatal(err)
				}
				if string(data) != "hello" {
					t.Fatalf("content %q", data)
				}
			},
		},
		{
			name: "overwrite",
			setup: func(h *host.Memory) {
				if err := h.WriteFile(dir+"/f.txt", []byte("old")); err != nil {
					t.Fatal(err)
				}
			},
			raw: `{"path":"f.txt","content":"new"}`,
			check: func(t *testing.T, h *host.Memory) {
				data, _ := h.ReadFile(dir + "/f.txt")
				if string(data) != "new" {
					t.Fatalf("got %q", data)
				}
			},
		},
		{
			name: "directory",
			setup: func(h *host.Memory) {
				if err := h.WriteFile(dir+"/d/x", []byte("x")); err != nil {
					t.Fatal(err)
				}
			},
			raw:     `{"path":"d","content":"x"}`,
			wantErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := host.NewMemory()
			if tc.setup != nil {
				tc.setup(h)
			}
			res, err := writeTool.Run(ctx, Env{Host: h, Dir: dir}, json.RawMessage(tc.raw))
			if err != nil {
				t.Fatal(err)
			}
			if res.IsError != tc.wantErr {
				t.Fatalf("IsError=%v text=%q", res.IsError, res.Text)
			}
			if tc.wantErr && !strings.Contains(res.Text, "directory") {
				t.Fatalf("text %q", res.Text)
			}
			if tc.check != nil {
				tc.check(t, h)
			}
		})
	}
}
