package skill

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/HoneyPaptan/hp-harness/internal/host"
)

func filler(n int) string {
	return "#" + strings.Repeat("x", n-2) + "\n"
}

func TestDiscoverReadsOnlyTheHeadWhenTheFrontmatterFits(t *testing.T) {
	body := strings.Repeat("body line\n", 5000)
	h := host.NewMemory()
	writeSkill(h, "/a", "big", "---\nname: big\ndescription: d\n---\n"+body)
	got, err := Discover(h, []string{"/a"})
	if err != nil || len(got) != 1 || got[0].Name != "big" || got[0].Description != "d" {
		t.Fatalf("got %+v, %v", got, err)
	}
}

func TestDiscoverFallsBackWhenTheFrontmatterRunsPastTheHead(t *testing.T) {
	long := strings.Repeat("word ", 2000)
	h := host.NewMemory()
	writeSkill(h, "/a", "wide", "---\nname: wide\ndescription: >\n  "+long+"\n---\nbody\n")
	got, err := Discover(h, []string{"/a"})
	if err != nil || len(got) != 1 || got[0].Name != "wide" {
		t.Fatalf("got %+v, %v", got, err)
	}
	if want := strings.TrimSpace(long); got[0].Description != strings.Join(strings.Fields(want), " ") {
		t.Fatalf("description cut: %d chars", len(got[0].Description))
	}
}

func TestDiscoverIgnoresADividerCutByTheHeadBoundary(t *testing.T) {
	prefix := "---\ndescription: d\n"
	text := prefix + filler(headSize-len(prefix)-3) + "---junk\nname: late\n---\nbody\n"
	if !strings.HasPrefix(text[headSize-3:], "---") {
		t.Fatal("setup: the divider does not start at the head boundary")
	}
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "cut"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "cut", "SKILL.md"), []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := Discover(host.Native{}, []string{dir})
	if err != nil || len(got) != 1 || got[0].Name != "late" {
		t.Fatalf("got %+v, %v", got, err)
	}
}
