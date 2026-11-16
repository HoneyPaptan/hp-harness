package tui

import (
	"strings"
	"testing"
)

func TestSessionPickerExplainsMissingKeys(t *testing.T) {
	a := newFakeAgent()
	a.model = ""
	a.keyHint = "add a key with /key NAME"
	h := startSessionWith(t, a, Config{PickModel: true})
	h.play([]step{{wait: "add a key with /key NAME"}, {wait: providerPrompt}, {keys: "\x1b"}, {keys: "\x04"}})
	h.waitExit()
}

func TestSessionKeySavedWithModelKeepsPromptClosed(t *testing.T) {
	a := newFakeAgent()
	h := startSession(t, a, false, false)
	h.play([]step{{keys: "/key GROQ_API_KEY\rs3cr3t\r"}, {wait: "saved GROQ_API_KEY"}, {keys: "\x04"}})
	h.waitExit()
	if strings.Contains(h.out.String(), providerPrompt) {
		t.Fatalf("picker opened with a model set: %q", h.out.String())
	}
}

func TestSessionQuitPrintsAgentNotes(t *testing.T) {
	a := newFakeAgent()
	a.quitNotes = []string{"run abc, hp -c continues it"}
	h := startSession(t, a, false, false)
	h.play([]step{{keys: "\x04"}})
	h.waitExit()
	if !strings.Contains(h.out.String(), "run abc, hp -c continues it") {
		t.Fatalf("no quit note: %q", h.out.String())
	}
}
