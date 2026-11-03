package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/HoneyPaptan/hp-harness/internal/host"
)

func TestExpandMentions(t *testing.T) {
	root := "/proj"
	tests := []struct {
		name   string
		setup  func(h *host.Memory)
		prompt string
		check  func(t *testing.T, got string)
	}{
		{
			name: "one file",
			setup: func(h *host.Memory) {
				_ = h.WriteFile(filepath.Join(root, "internal/cli/model.go"), []byte("package cli\n"))
			},
			prompt: "explain @internal/cli/model.go",
			check: func(t *testing.T, got string) {
				t.Helper()
				wantHead := "explain @internal/cli/model.go\n\n<file path=\"internal/cli/model.go\">\n```\npackage cli\n```\n</file>"
				if got != wantHead {
					t.Fatalf("got %q", got)
				}
			},
		},
		{
			name: "two files duplicate once",
			setup: func(h *host.Memory) {
				_ = h.WriteFile(filepath.Join(root, "a.go"), []byte("a\n"))
				_ = h.WriteFile(filepath.Join(root, "b.go"), []byte("b\n"))
			},
			prompt: "see @a.go and @b.go again @a.go",
			check: func(t *testing.T, got string) {
				t.Helper()
				if strings.Count(got, `<file path="a.go">`) != 1 {
					t.Fatalf("a.go blocks %d", strings.Count(got, `<file path="a.go">`))
				}
				if strings.Count(got, `<file path="b.go">`) != 1 {
					t.Fatalf("b.go blocks %d", strings.Count(got, `<file path="b.go">`))
				}
			},
		},
		{
			name: "trailing punctuation",
			setup: func(h *host.Memory) {
				_ = h.WriteFile(filepath.Join(root, "a.go"), []byte("x\n"))
			},
			prompt: "read @a.go, ok",
			check: func(t *testing.T, got string) {
				t.Helper()
				if !strings.Contains(got, `<file path="a.go">`) {
					t.Fatalf("got %q", got)
				}
			},
		},
		{
			name:   "parent escape",
			prompt: "no @../secret",
			check: func(t *testing.T, got string) {
				t.Helper()
				if got != "no @../secret" {
					t.Fatalf("got %q", got)
				}
			},
		},
		{
			name:   "absolute path",
			prompt: "no @/etc/passwd",
			check: func(t *testing.T, got string) {
				t.Helper()
				if got != "no @/etc/passwd" {
					t.Fatalf("got %q", got)
				}
			},
		},
		{
			name:   "dotdot component",
			prompt: "no @a/../../x",
			check: func(t *testing.T, got string) {
				t.Helper()
				if got != "no @a/../../x" {
					t.Fatalf("got %q", got)
				}
			},
		},
		{
			name: "symlink file",
			setup: func(h *host.Memory) {
				_ = h.WriteFile(filepath.Join(root, "real.go"), []byte("x\n"))
				h.Link(filepath.Join(root, "linked.go"), filepath.Join(root, "real.go"))
			},
			prompt: "see @linked.go",
			check: func(t *testing.T, got string) {
				t.Helper()
				if got != "see @linked.go" {
					t.Fatalf("got %q", got)
				}
			},
		},
		{
			name: "symlink parent dir",
			setup: func(h *host.Memory) {
				_ = h.WriteFile(filepath.Join(root, "real", "f.go"), []byte("x\n"))
				h.Link(filepath.Join(root, "linked"), filepath.Join(root, "real"))
			},
			prompt: "see @linked/f.go",
			check: func(t *testing.T, got string) {
				t.Helper()
				if got != "see @linked/f.go" {
					t.Fatalf("got %q", got)
				}
			},
		},
		{
			name: "directory",
			setup: func(h *host.Memory) {
				_ = h.WriteFile(filepath.Join(root, "sub", ".keep"), []byte("x\n"))
			},
			prompt: "see @sub",
			check: func(t *testing.T, got string) {
				t.Helper()
				if got != "see @sub" {
					t.Fatalf("got %q", got)
				}
			},
		},
		{
			name:   "missing file",
			prompt: "see @missing.go",
			check: func(t *testing.T, got string) {
				t.Helper()
				if got != "see @missing.go" {
					t.Fatalf("got %q", got)
				}
			},
		},
		{
			name: "binary",
			setup: func(h *host.Memory) {
				_ = h.WriteFile(filepath.Join(root, "bin.dat"), []byte{0, 1, 2})
			},
			prompt: "see @bin.dat",
			check: func(t *testing.T, got string) {
				t.Helper()
				if got != "see @bin.dat" {
					t.Fatalf("got %q", got)
				}
			},
		},
		{
			name: "truncate per file",
			setup: func(h *host.Memory) {
				_ = h.WriteFile(filepath.Join(root, "big.txt"), []byte(strings.Repeat("x", 70*1024)))
			},
			prompt: "see @big.txt",
			check: func(t *testing.T, got string) {
				t.Helper()
				if !strings.Contains(got, "[truncated at 64 KB]") {
					t.Fatal("missing truncate marker")
				}
			},
		},
		{
			name: "total cap",
			setup: func(h *host.Memory) {
				for i := range 5 {
					name := filepath.Join(root, string(rune('a'+i))+".txt")
					_ = h.WriteFile(name, []byte(strings.Repeat("y", 64*1024)))
				}
			},
			prompt: "see @a.txt @b.txt @c.txt @d.txt @e.txt",
			check: func(t *testing.T, got string) {
				t.Helper()
				if !strings.Contains(got, "[skipped @e.txt: message attachment limit]") {
					t.Fatalf("got %q", got)
				}
				if strings.Contains(got, `<file path="e.txt">`) {
					t.Fatal("attached e.txt")
				}
			},
		},
		{
			name: "longer fence",
			setup: func(h *host.Memory) {
				_ = h.WriteFile(filepath.Join(root, "fenced.md"), []byte("line\n```\nmore\n"))
			},
			prompt: "see @fenced.md",
			check: func(t *testing.T, got string) {
				t.Helper()
				if !strings.Contains(got, "````\nline\n```\nmore\n````") {
					t.Fatalf("got %q", got)
				}
			},
		},
		{
			name:   "no mentions",
			prompt: "plain text",
			check: func(t *testing.T, got string) {
				t.Helper()
				if got != "plain text" {
					t.Fatalf("got %q", got)
				}
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := host.NewMemory()
			if tc.setup != nil {
				tc.setup(h)
			}
			got := expandMentions(h, root, tc.prompt)
			tc.check(t, got)
		})
	}
}
