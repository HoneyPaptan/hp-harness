package prompt

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/HoneyPaptan/hp-harness/internal/skill"
)

func TestBuildNoSkills(t *testing.T) {
	fixed := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	got := Build(Info{Dir: "/proj", Date: fixed})
	want := baseText + "\nWorking directory: /proj\nDate: 2026-03-15\n"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	if len(got) >= 1024 {
		t.Fatalf("len %d", len(got))
	}
}

func TestBuildInstructionsSection(t *testing.T) {
	fixed := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	got := Build(Info{
		Dir: "/proj", Date: fixed,
		Instructions: []Instruction{
			{Path: "~/.config/hp/AGENTS.md", Text: "be nice\n"},
		},
	})
	want := baseText + "\nWorking directory: /proj\nDate: 2026-03-15\n\n\nProject instructions (from AGENTS.md files; the user's messages take priority, and a file closer to the working directory wins on conflicts):\n<instructions path=\"~/.config/hp/AGENTS.md\">\nbe nice\n\n</instructions>\n"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestBuildNoInstructionsSection(t *testing.T) {
	fixed := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	got := Build(Info{Dir: "/proj", Date: fixed, Instructions: nil})
	if strings.Contains(got, "Project instructions") {
		t.Fatal(got)
	}
}

func TestBuildTwoSkills(t *testing.T) {
	fixed := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	got := Build(Info{
		Dir: "/w", Date: fixed,
		Skills: []skill.Skill{
			{Name: "alpha", Description: "First skill."},
			{Name: "beta", Description: "Second."},
		},
	})
	if !strings.Contains(got, "- alpha: First skill.\n") {
		t.Fatal(got)
	}
	if !strings.Contains(got, "- beta: Second.\n") {
		t.Fatal(got)
	}
	if !strings.Contains(got, "Skills (call the skill tool") {
		t.Fatal(got)
	}
}

func TestBuildCutMultibyte(t *testing.T) {
	desc := strings.Repeat("日", 210)
	got := cutDesc(desc, 200)
	r := []rune(got)
	if len(r) != 200 {
		t.Fatalf("runes %d", len(r))
	}
	if !strings.HasSuffix(got, "...") {
		t.Fatal(got)
	}
	if !utf8.ValidString(got) {
		t.Fatal("invalid utf8")
	}
}

func TestBuildBrowser(t *testing.T) {
	got := Build(Info{Dir: "/w", Date: time.Unix(0, 0), Browser: true})
	if strings.Contains(got, "terminal") || !strings.Contains(got, "no shell") || !strings.Contains(got, "write it as a Markdown file") {
		t.Fatalf("%s", got)
	}
}
