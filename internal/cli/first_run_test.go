package cli

import (
	"errors"
	"strings"
	"testing"

	"github.com/HoneyPaptan/hp-harness/internal/host"
	"github.com/HoneyPaptan/hp-harness/internal/provider/fake"
)

func TestKeyHintNamesRealKeys(t *testing.T) {
	known := map[string]bool{}
	for _, def := range presets {
		known[def.keyEnv] = true
	}
	for _, name := range []string{"GROQ_API_KEY", "OPENCODE_API_KEY", "OPENROUTER_API_KEY"} {
		if !known[name] || !strings.Contains(keyHint, name) {
			t.Fatalf("%s is not a preset key named in the hint", name)
		}
	}
}

func TestSessionFirstRunWithoutKeyExplainsAndReopensPicker(t *testing.T) {
	fp := &fake.Provider{}
	home := t.TempDir()
	env, _, _ := testEnv(t, host.NewMemory(), fp, map[string]string{"XAI_API_KEY": "", "HOME": home, "XDG_CONFIG_HOME": home})
	term := newFakeTerm(t)
	env.Term = term
	env.Args = nil
	code := startHP(t, env)
	term.await(t, "provider: ", 1)
	term.await(t, "hp key set NAME", 1)
	term.send(t, "\x1b")
	term.await(t, "model unchanged", 1)
	before := strings.Count(term.text(), "provider: ")
	term.send(t, "/key GROQ_API_KEY\r")
	term.send(t, "s3cr3t\r")
	term.await(t, "saved GROQ_API_KEY", 1)
	term.await(t, "groq", 1)
	if strings.Count(term.text(), "provider: ") <= before {
		t.Fatal("picker did not reopen after the key was saved")
	}
	term.send(t, "\x1b")
	term.send(t, "\x04")
	awaitCode(t, code)
}

func TestSessionQuitPrintsRunHint(t *testing.T) {
	fp := &fake.Provider{Turns: []fake.Turn{fake.TextTurn("one")}}
	env, _, _ := testEnv(t, host.NewMemory(), fp, nil)
	term := newFakeTerm(t)
	env.Term = term
	env.Args = []string{"-m", "xai/grok"}
	code := startHP(t, env)
	term.send(t, "first\r")
	term.await(t, "0 steps", 1)
	term.send(t, "\x04")
	awaitCode(t, code)
	ids, _ := sortedRunIDs(env, runsDir(env.Getenv))
	if len(ids) != 1 {
		t.Fatalf("runs %v", ids)
	}
	out := term.text()
	if !strings.Contains(out, "run "+ids[0]) || !strings.Contains(out, "hp -c") {
		t.Fatalf("no run hint in %q", out)
	}
	if strings.Contains(out, "hp resume") {
		t.Fatalf("finished run offers hp resume: %q", out)
	}
}

func TestSessionQuitAfterStoppedTurnOffersResume(t *testing.T) {
	fp := &fake.Provider{Turns: []fake.Turn{{Err: errors.New("boom")}}}
	env, _, _ := testEnv(t, host.NewMemory(), fp, nil)
	term := newFakeTerm(t)
	env.Term = term
	env.Args = []string{"-m", "xai/grok"}
	code := startHP(t, env)
	term.send(t, "first\r")
	term.await(t, "error: boom", 1)
	term.send(t, "\x04")
	awaitCode(t, code)
	ids, _ := sortedRunIDs(env, runsDir(env.Getenv))
	if out := term.text(); len(ids) != 1 || !strings.Contains(out, "hp resume "+ids[0]) {
		t.Fatalf("no resume hint in %q", out)
	}
}

func TestSessionQuitWithoutRunPrintsNothing(t *testing.T) {
	env, _, _ := testEnv(t, host.NewMemory(), &fake.Provider{}, nil)
	term := newFakeTerm(t)
	env.Term = term
	env.Args = []string{"-m", "xai/grok"}
	code := startHP(t, env)
	term.await(t, "› ", 1)
	term.send(t, "\x04")
	awaitCode(t, code)
	if out := term.text(); strings.Contains(out, "hp -c") {
		t.Fatalf("hint without a run: %q", out)
	}
}
