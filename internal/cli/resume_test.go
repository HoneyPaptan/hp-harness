package cli

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/HoneyPaptan/hp-harness/internal/host"
	"github.com/HoneyPaptan/hp-harness/internal/journal"
	"github.com/HoneyPaptan/hp-harness/internal/provider"
	"github.com/HoneyPaptan/hp-harness/internal/provider/fake"
)

func TestResumeInterrupted(t *testing.T) {
	h := host.NewMemory()
	block := blockProvider{}
	env, stdout, stderr := testEnv(t, h, nil, nil)
	env.NewProvider = func(Endpoint) provider.Provider { return block }
	env.Args = []string{"run", "-m", "xai/grok", "go"}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(5 * time.Millisecond)
		cancel()
	}()
	if Main(ctx, env) != 130 {
		t.Fatal(stderr.String())
	}
	id := parseResumeID(stderr.String())
	fp := &fake.Provider{Turns: []fake.Turn{fake.TextTurn("finished")}}
	env.NewProvider = func(Endpoint) provider.Provider { return fp }
	env.Args = []string{"resume", id}
	stdout.Reset()
	stderr.Reset()
	if code := Main(context.Background(), env); code != 0 {
		t.Fatalf("code %d %s", code, stderr.String())
	}
	if stdout.String() != "finished\n" {
		t.Fatalf("stdout %q", stdout.String())
	}
}

func parseResumeID(s string) string {
	const p = "hp resume "
	i := strings.Index(s, p)
	if i < 0 {
		return ""
	}
	rest := s[i+len(p):]
	if j := strings.IndexByte(rest, '\n'); j >= 0 {
		rest = rest[:j]
	}
	return strings.TrimSpace(rest)
}

func TestResumeFinished(t *testing.T) {
	h := host.NewMemory()
	fp := &fake.Provider{Turns: []fake.Turn{fake.TextTurn("ok")}}
	env, _, stderr := testEnv(t, h, fp, nil)
	env.Args = []string{"run", "-m", "xai/grok", "x"}
	if Main(context.Background(), env) != 0 {
		t.Fatal(stderr.String())
	}
	id := parseResumeID(stderr.String())
	if id == "" {
		id = listFirstRunID(t, h, env.Getenv)
	}
	env.Args = []string{"resume", id}
	if code := Main(context.Background(), env); code != 1 {
		t.Fatalf("code %d", code)
	}
	if !strings.Contains(stderr.String(), "already finished") {
		t.Fatal(stderr.String())
	}
}

func listFirstRunID(t *testing.T, h *host.Memory, getenv func(string) string) string {
	t.Helper()
	dir := runsDir(getenv)
	ents, err := h.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(ents) == 0 {
		t.Fatal("no runs")
	}
	return strings.TrimSuffix(ents[0].Name, ".jsonl")
}

func TestResumeMissingID(t *testing.T) {
	h := host.NewMemory()
	env, _, _ := testEnv(t, h, nil, nil)
	env.Args = []string{"resume"}
	if Main(context.Background(), env) != 2 {
		t.Fatal()
	}
}

func seedRun(t *testing.T, h *host.Memory, getenv func(string) string, id, prompt, reason string, done bool) {
	t.Helper()
	jpath := filepath.Join(runsDir(getenv), id+".jsonl")
	start := journal.RunStart{
		RunID: id, Model: "m", System: "s", Dir: "/w", StepLimit: 50,
		Preset: "xai", BaseURL: "https://api.x.ai/v1", KeyEnv: "XAI_API_KEY",
	}
	w, err := journal.Create(h, jpath, start)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.User(prompt); err != nil {
		t.Fatal(err)
	}
	if done {
		if err := w.End(reason); err != nil {
			t.Fatal(err)
		}
	}
}

func TestResumeInterruptedAgain(t *testing.T) {
	h := host.NewMemory()
	cancelled := fake.Turn{Events: []provider.Event{{Kind: provider.EventText, Text: "par"}}, Err: context.Canceled}
	fp := &fake.Provider{Turns: []fake.Turn{cancelled, cancelled}}
	env, _, stderr := testEnv(t, h, fp, nil)
	env.Args = []string{"run", "-m", "xai/grok", "go"}
	if Main(context.Background(), env) != 130 {
		t.Fatal(stderr.String())
	}
	id := parseResumeID(stderr.String())
	stderr.Reset()
	env.Args = []string{"resume", id}
	if code := Main(context.Background(), env); code != 130 {
		t.Fatalf("code %d %s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "interrupted, resume with: hp resume "+id) {
		t.Fatal(stderr.String())
	}
}
