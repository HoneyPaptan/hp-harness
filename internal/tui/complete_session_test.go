package tui

import (
	"strings"
	"testing"
)

func TestSessionSlashCompletion(t *testing.T) {
	a := newFakeAgent()
	h := startSession(t, a, false, false)
	h.play([]step{
		{keys: "/mo\t"},
		{wait: "› /model "},
		{keys: "\x15/quit \r"},
	})
	h.waitExit()
}

func TestSessionSlashCompletionEnter(t *testing.T) {
	a := newFakeAgent()
	h := startSession(t, a, false, false)
	h.play([]step{
		{keys: "/"},
		{wait: "› help"},
		{keys: "\x1b[B\r"},
		{wait: "› /model "},
		{keys: "\x15/quit \r"},
	})
	h.waitExit()
	if len(a.sent()) != 0 {
		t.Fatalf("submitted %q", a.sent())
	}
}

func TestSessionSlashUnknownStillSubmits(t *testing.T) {
	a := newFakeAgent()
	h := startSession(t, a, false, false)
	h.play([]step{
		{keys: "/zz\r"},
		{wait: "unknown command /zz"},
		{keys: "\x04"},
	})
	h.waitExit()
}

func TestSessionSlashCompletionEsc(t *testing.T) {
	a := newFakeAgent()
	h := startSession(t, a, false, false)
	h.play([]step{
		{keys: "/mo"},
		{wait: "model"},
		{keys: "\x1b"},
		{wait: "› /mo"},
		{keys: "\x15/quit \r"},
	})
	h.waitExit()
}

func TestSessionFileCompletion(t *testing.T) {
	a := newFakeAgent()
	a.files = []string{"internal/cli/model.go", "README.md"}
	h := startSession(t, a, false, false)
	h.play([]step{
		{keys: "fix @mod\t"},
		{wait: "› fix @internal/cli/model.go "},
		{keys: "x"},
		{keys: "x"},
		{keys: "\x15/quit \r"},
	})
	h.waitExit()
	if a.filesCalls != 1 {
		t.Fatalf("Files calls %d want 1", a.filesCalls)
	}
}

func TestSessionHelpOutputUnchanged(t *testing.T) {
	a := newFakeAgent()
	h := startSession(t, a, false, false)
	h.play([]step{{keys: "/help \r"}, {wait: "/quit"}, {keys: "/quit \r"}})
	h.waitExit()
	out := h.out.String()
	for _, line := range helpLines[:9] {
		if !strings.Contains(out, line) {
			t.Fatalf("missing %q in %q", line, out)
		}
	}
}

func TestSessionSlashExactMatchSubmits(t *testing.T) {
	a := newFakeAgent()
	h := startSession(t, a, false, false)
	h.play([]step{
		{keys: "/help\r"},
		{wait: "this list"},
		{keys: "\x04"},
	})
	h.waitExit()
}
