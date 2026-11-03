package cli

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/HoneyPaptan/hp-harness/internal/host"
	"github.com/HoneyPaptan/hp-harness/internal/provider"
	"github.com/HoneyPaptan/hp-harness/internal/tool"
)

func nestedToolset(t *testing.T, h *host.Memory, workDir string, st settings, msgs []provider.Message) *toolset {
	t.Helper()
	env, _, _ := testEnv(t, h, nil, map[string]string{"HOME": "/home/t"})
	return openToolset(env, workDir, nil, nil, st, msgs)
}

func runNestedRead(t *testing.T, set *toolset, h *host.Memory, workDir, path string) tool.Result {
	t.Helper()
	read, ok := set.reg.Get("read")
	if !ok {
		t.Fatal("read missing")
	}
	raw, err := json.Marshal(map[string]string{"path": path})
	if err != nil {
		t.Fatal(err)
	}
	res, err := read.Run(context.Background(), tool.Env{Host: h, Dir: workDir}, raw)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestNestedAttachOnRead(t *testing.T) {
	h := host.NewMemory()
	work := "/proj"
	writeAgents(t, h, filepath.Join(work, "apps", "web", "AGENTS.md"), "web rules\n")
	_ = h.WriteFile(filepath.Join(work, "apps", "web", "a.ts"), []byte("a\n"))
	set := nestedToolset(t, h, work, settings{}, nil)
	res := runNestedRead(t, set, h, work, "apps/web/a.ts")
	if !strings.Contains(res.Text, `<instructions path="apps/web/AGENTS.md">`) {
		t.Fatalf("%q", res.Text)
	}
	_ = h.WriteFile(filepath.Join(work, "apps", "web", "b.ts"), []byte("b\n"))
	res2 := runNestedRead(t, set, h, work, "apps/web/b.ts")
	if strings.Contains(res2.Text, "<instructions path=") {
		t.Fatalf("%q", res2.Text)
	}
}

func TestNestedAttachNearest(t *testing.T) {
	h := host.NewMemory()
	work := "/proj"
	writeAgents(t, h, filepath.Join(work, "apps", "web", "AGENTS.md"), "outer\n")
	writeAgents(t, h, filepath.Join(work, "apps", "web", "src", "AGENTS.md"), "inner\n")
	_ = h.WriteFile(filepath.Join(work, "apps", "web", "src", "x.ts"), []byte("x\n"))
	res := runNestedRead(t, nestedToolset(t, h, work, settings{}, nil), h, work, "apps/web/src/x.ts")
	if !strings.Contains(res.Text, `<instructions path="apps/web/src/AGENTS.md">`) {
		t.Fatalf("%q", res.Text)
	}
	if strings.Contains(res.Text, `path="apps/web/AGENTS.md"`) {
		t.Fatal(res.Text)
	}
}

func TestNestedAttachSkipsError(t *testing.T) {
	h := host.NewMemory()
	res := runNestedRead(t, nestedToolset(t, h, "/proj", settings{}, nil), h, "/proj", "missing.go")
	if strings.Contains(res.Text, "<instructions path=") {
		t.Fatal(res.Text)
	}
}

func TestNestedAttachOutsideWorkDir(t *testing.T) {
	h := host.NewMemory()
	work := "/proj"
	writeAgents(t, h, "/outside/AGENTS.md", "secret\n")
	_ = h.WriteFile("/outside/x.go", []byte("x\n"))
	env, _, _ := testEnv(t, h, nil, map[string]string{"HOME": "/home/t"})
	set := openToolset(env, work, nil, nil, settings{}, nil)
	read, _ := set.reg.Get("read")
	raw, _ := json.Marshal(map[string]string{"path": "/outside/x.go"})
	res, err := read.Run(context.Background(), tool.Env{Host: h, Dir: work}, raw)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(res.Text, "<instructions path=") {
		t.Fatal(res.Text)
	}
}

func TestNestedAttachSkipsSymlinkAgents(t *testing.T) {
	h := host.NewMemory()
	work := "/proj"
	target := filepath.Join(work, "apps", "real.md")
	link := filepath.Join(work, "apps", "AGENTS.md")
	writeAgents(t, h, target, "real\n")
	h.Link(link, target)
	_ = h.WriteFile(filepath.Join(work, "apps", "a.ts"), []byte("a\n"))
	res := runNestedRead(t, nestedToolset(t, h, work, settings{}, nil), h, work, "apps/a.ts")
	if strings.Contains(res.Text, "<instructions path=") {
		t.Fatal(res.Text)
	}
}

func TestNestedRebuildAttached(t *testing.T) {
	h := host.NewMemory()
	work := "/proj"
	writeAgents(t, h, filepath.Join(work, "apps", "web", "AGENTS.md"), "web rules\n")
	_ = h.WriteFile(filepath.Join(work, "apps", "web", "a.ts"), []byte("a\n"))
	msgs := []provider.Message{{
		ToolResults: []provider.ToolResult{{
			Text: "file\n\n<instructions path=\"apps/web/AGENTS.md\">\nweb rules\n\n</instructions>",
		}},
	}}
	res := runNestedRead(t, nestedToolset(t, h, work, settings{}, msgs), h, work, "apps/web/a.ts")
	if strings.Contains(res.Text, "<instructions path=") {
		t.Fatalf("%q", res.Text)
	}
}

func TestNestedContextDisabled(t *testing.T) {
	h := host.NewMemory()
	work := "/proj"
	writeAgents(t, h, filepath.Join(work, "a", "AGENTS.md"), "rules\n")
	_ = h.WriteFile(filepath.Join(work, "a", "f.go"), []byte("f\n"))
	off := false
	res := runNestedRead(t, nestedToolset(t, h, work, settings{Context: &off}, nil), h, work, "a/f.go")
	if strings.Contains(res.Text, "<instructions path=") {
		t.Fatal(res.Text)
	}
}

func TestAttachedInstructionPaths(t *testing.T) {
	got := attachedInstructionPaths([]provider.Message{{
		ToolResults: []provider.ToolResult{{
			Text: "x\n\n<instructions path=\"apps/web/AGENTS.md\">\nbody\n</instructions>",
		}},
	}})
	if _, ok := got["apps/web/AGENTS.md"]; !ok {
		t.Fatalf("%v", got)
	}
}
