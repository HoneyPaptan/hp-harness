package cli

import (
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/HoneyPaptan/hp-harness/internal/host"
)

func writeAgents(t *testing.T, h *host.Memory, path, body string) {
	t.Helper()
	if err := h.WriteFile(path, []byte(body)); err != nil {
		t.Fatal(err)
	}
}

func TestLoadInstructionsOrder(t *testing.T) {
	h := host.NewMemory()
	home := "/home/t"
	root := filepath.Join(home, "proj")
	writeAgents(t, h, filepath.Join(home, ".config", "hp", "AGENTS.md"), "personal\n")
	writeAgents(t, h, filepath.Join(root, "AGENTS.md"), "outer\n")
	writeAgents(t, h, filepath.Join(root, "inner", "AGENTS.md"), "inner\n")
	dir := filepath.Join(root, "inner", "work")
	got := loadInstructions(h, dir, home)
	if len(got) != 3 {
		t.Fatalf("len %d", len(got))
	}
	if got[0].Text != "personal\n" || got[0].Path != "~/.config/hp/AGENTS.md" {
		t.Fatalf("personal %#v", got[0])
	}
	if got[1].Text != "outer\n" {
		t.Fatalf("outer %#v", got[1])
	}
	if got[2].Text != "inner\n" {
		t.Fatalf("inner %#v", got[2])
	}
}

func TestLoadInstructionsSkipsHomeDir(t *testing.T) {
	h := host.NewMemory()
	home := "/home/t"
	writeAgents(t, h, filepath.Join(home, "AGENTS.md"), "home\n")
	got := loadInstructions(h, home, home)
	if len(got) != 0 {
		t.Fatalf("got %#v", got)
	}
}

func TestLoadInstructionsOutsideHome(t *testing.T) {
	h := host.NewMemory()
	home := "/home/t"
	dir := "/srv/app"
	writeAgents(t, h, filepath.Join(home, ".config", "hp", "AGENTS.md"), "personal\n")
	writeAgents(t, h, filepath.Join(home, "proj", "AGENTS.md"), "under\n")
	writeAgents(t, h, filepath.Join(dir, "AGENTS.md"), "only\n")
	got := loadInstructions(h, dir, home)
	if len(got) != 2 {
		t.Fatalf("len %d", len(got))
	}
	if got[0].Text != "personal\n" {
		t.Fatalf("personal %#v", got[0])
	}
	if got[1].Text != "only\n" || got[1].Path != "/srv/app/AGENTS.md" {
		t.Fatalf("dir %#v", got[1])
	}
}

func TestLoadInstructionsMissingFiles(t *testing.T) {
	h := host.NewMemory()
	got := loadInstructions(h, "/home/t/proj", "/home/t")
	if len(got) != 0 {
		t.Fatalf("got %#v", got)
	}
}

func TestLoadInstructionsSkipsSymlink(t *testing.T) {
	h := host.NewMemory()
	root := "/home/t/proj"
	target := filepath.Join(root, "real.md")
	link := filepath.Join(root, "AGENTS.md")
	writeAgents(t, h, target, "real\n")
	h.Link(link, target)
	got := loadInstructions(h, root, "/home/t")
	if len(got) != 0 {
		t.Fatalf("got %#v", got)
	}
}

func TestLoadInstructionsTruncatesFile(t *testing.T) {
	h := host.NewMemory()
	body := strings.Repeat("a", instructionFileMaxRunes+10)
	writeAgents(t, h, filepath.Join("/home/t", "proj", "AGENTS.md"), body)
	got := loadInstructions(h, filepath.Join("/home/t", "proj"), "/home/t")
	if len(got) != 1 {
		t.Fatalf("len %d", len(got))
	}
	if !strings.HasSuffix(got[0].Text, instructionTruncatedSuffix) {
		t.Fatalf("suffix missing len %d", len(got[0].Text))
	}
	if !utf8.ValidString(got[0].Text) {
		t.Fatal("invalid utf8")
	}
}

func TestLoadInstructionsTotalLimit(t *testing.T) {
	h := host.NewMemory()
	home := "/home/t"
	root := filepath.Join(home, "a", "b", "c")
	chunk := strings.Repeat("x", instructionTotalMaxBytes/2)
	writeAgents(t, h, filepath.Join(home, "a", "AGENTS.md"), chunk)
	writeAgents(t, h, filepath.Join(home, "a", "b", "AGENTS.md"), chunk)
	writeAgents(t, h, filepath.Join(root, "AGENTS.md"), "tail\n")
	got := loadInstructions(h, root, home)
	if len(got) != 3 {
		t.Fatalf("len %d", len(got))
	}
	if got[2].Text != instructionSkipMarker {
		t.Fatalf("third %#v", got[2])
	}
}

func TestContextDisabledNoInstructions(t *testing.T) {
	h := host.NewMemory()
	writeAgents(t, h, filepath.Join("/home/t", "proj", "AGENTS.md"), "x\n")
	off := false
	st := settings{Context: &off}
	if contextEnabled(st) {
		t.Fatal("expected disabled")
	}
	env, _, _ := testEnv(t, h, nil, map[string]string{"HOME": "/home/t"})
	sys := buildSystem(env, filepath.Join("/home/t", "proj"), nil, st)
	if strings.Contains(sys, "Project instructions") {
		t.Fatal(sys)
	}
}
