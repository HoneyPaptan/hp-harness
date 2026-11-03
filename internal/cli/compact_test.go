package cli

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/HoneyPaptan/hp-harness/internal/host"
	"github.com/HoneyPaptan/hp-harness/internal/journal"
	"github.com/HoneyPaptan/hp-harness/internal/provider"
	"github.com/HoneyPaptan/hp-harness/internal/provider/fake"
	"github.com/HoneyPaptan/hp-harness/internal/tui"
)

func compactSettingsEnv(t *testing.T, fp *fake.Provider) (*sessionAgent, *host.Memory) {
	t.Helper()
	h := host.NewMemory()
	h.WriteFile("/proj/f.txt", []byte("x"))
	env, _, _ := testEnv(t, h, fp, map[string]string{"HOME": t.TempDir()})
	writeSettingsFile(t, env.Getenv("HOME"), `{"context_windows":{"xai/grok":1000},"auto_compact_percent":80}`)
	ep := Endpoint{Preset: "xai", Model: "grok", BaseURL: presets["xai"].baseURL, KeyEnv: "XAI_API_KEY"}
	spec := endpointSpec(ep)
	a, err := newSessionAgent(env, runOpts{}, ep, map[string]Endpoint{spec: ep}, nil, "/proj", nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { a.release() })
	return a, h
}

func fatReadTurn(usage int) fake.Turn {
	call := provider.ToolCall{ID: "1", Name: "read", Args: json.RawMessage(`{"path":"f.txt"}`)}
	return fake.Turn{Events: []provider.Event{
		{Kind: provider.EventText, Text: strings.Repeat("y", 800)},
		{Kind: provider.EventToolCall, Call: call},
		{Kind: provider.EventDone, StopReason: "tool_use", Usage: provider.Usage{InputTokens: usage, OutputTokens: 0}},
	}}
}

func TestSessionAgentCompactNoRun(t *testing.T) {
	fp := &fake.Provider{Turns: []fake.Turn{fake.TextTurn("x")}}
	a, _ := compactSettingsEnv(t, fp)
	ok, err := a.Compact(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("expected false")
	}
	if len(fp.Requests) != 0 {
		t.Fatalf("requests %d", len(fp.Requests))
	}
}

func TestSessionAgentCompactAfterTurns(t *testing.T) {
	long := strings.Repeat("x", 800)
	fp := &fake.Provider{Turns: []fake.Turn{
		fake.TextTurn(long),
		fake.TextTurn(long),
		fake.TextTurn("summary"),
	}}
	a, h := compactSettingsEnv(t, fp)
	if _, err := a.Send(context.Background(), strings.Repeat("q", 100), func(tui.Event) {}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Send(context.Background(), "again", func(tui.Event) {}); err != nil {
		t.Fatal(err)
	}
	before := len(fp.Requests)
	ok, err := a.Compact(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected compact")
	}
	if len(fp.Requests) != before+1 {
		t.Fatalf("requests %d before %d", len(fp.Requests), before)
	}
	recs, _ := journal.Read(h, a.jpath)
	for _, rec := range recs {
		if rec.Type == journal.TypeCompact {
			return
		}
	}
	t.Fatal("no compact record")
}

func TestRunJournalCompactFromSettings(t *testing.T) {
	h := host.NewMemory()
	h.WriteFile("/w/f.txt", []byte("x"))
	fp := &fake.Provider{Turns: []fake.Turn{
		fatReadTurn(100),
		fatReadTurn(900),
		fake.TextTurn("summary"),
		fake.TextTurn("done"),
	}}
	home := t.TempDir()
	env, _, stderr := testEnv(t, h, fp, map[string]string{"HOME": home})
	writeSettingsFile(t, home, `{"context_windows":{"xai/grok":1000},"auto_compact_percent":10}`)
	env.Args = []string{"run", "-m", "xai/grok", "-C", "/w", strings.Repeat("q", 100)}
	if code := Main(context.Background(), env); code != 0 {
		t.Fatalf("code %d %s", code, stderr.String())
	}
	ids, err := sortedRunIDs(env, runsDir(env.Getenv))
	if err != nil || len(ids) != 1 {
		t.Fatalf("runs %v err %v", ids, err)
	}
	jpath := runsDir(env.Getenv) + "/" + ids[0] + ".jsonl"
	recs, err := journal.Read(h, jpath)
	if err != nil {
		t.Fatal(err)
	}
	for _, rec := range recs {
		if rec.Type == journal.TypeCompact {
			return
		}
	}
	t.Fatal("no compact record")
}
