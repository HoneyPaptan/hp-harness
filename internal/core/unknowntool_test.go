package core

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/HoneyPaptan/hp-harness/internal/host"
	"github.com/HoneyPaptan/hp-harness/internal/journal"
	"github.com/HoneyPaptan/hp-harness/internal/provider"
	"github.com/HoneyPaptan/hp-harness/internal/provider/fake"
	"github.com/HoneyPaptan/hp-harness/internal/tool"
)

func unknownToolTurn(name string) fake.Turn {
	return fake.Turn{Err: &provider.UnknownToolError{Name: name, Body: "attempted to call tool '" + name + "' which was not in request.tools"}}
}

func unknownToolCfg() Config {
	return Config{RunID: "r", Model: "m", System: "s", Dir: "/w", JournalPath: "/j", Prompt: "list files", StepLimit: 5}
}

func unknownToolDeps(h host.Host, fp *fake.Provider, obs Observer) Deps {
	return Deps{Host: h, Provider: fp, Tools: tool.NewRegistry(tool.Builtins()...), Observer: obs}
}

func noteTexts(t *testing.T, h host.Host) []string {
	t.Helper()
	recs, err := journal.Read(h, "/j")
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, rec := range recs {
		if rec.Type != journal.TypeToolNote {
			continue
		}
		var n journal.ToolNote
		if err := json.Unmarshal(rec.Data, &n); err != nil {
			t.Fatal(err)
		}
		out = append(out, n.Text)
	}
	return out
}

func journalTypes(t *testing.T, h host.Host) []journal.Type {
	t.Helper()
	recs, err := journal.Read(h, "/j")
	if err != nil {
		t.Fatal(err)
	}
	out := make([]journal.Type, len(recs))
	for i, rec := range recs {
		out[i] = rec.Type
	}
	return out
}

func TestUnknownToolNote(t *testing.T) {
	defs := []provider.ToolDef{{Name: "read"}, {Name: "grep"}}
	tests := []struct {
		name string
		call string
		want string
	}{
		{"named", "ls", `Your last reply called a tool named "ls", which does not exist, so it was discarded. Use only these tools: read, grep. Redo that step.`},
		{"name could not be parsed", "", "Your last reply called a tool that does not exist, so it was discarded. Use only these tools: read, grep. Redo that step."},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := unknownToolNote(tc.call, defs); got != tc.want {
				t.Fatalf("note %q want %q", got, tc.want)
			}
		})
	}
}

func TestUnknownToolRecovers(t *testing.T) {
	h := host.NewMemory()
	fp := &fake.Provider{Turns: []fake.Turn{unknownToolTurn("ls"), fake.TextTurn("done")}}
	var starts []provider.ToolCall
	var results []provider.ToolResult
	retried := false
	obs := Observer{
		OnToolStart:  func(c provider.ToolCall) { starts = append(starts, c) },
		OnToolResult: func(r provider.ToolResult) { results = append(results, r) },
		OnRetry:      func(time.Duration, int, int, error) { retried = true },
	}
	out, err := Run(context.Background(), unknownToolDeps(h, fp, obs), unknownToolCfg())
	if err != nil {
		t.Fatal(err)
	}
	if out.Text != "done" || out.Reason != ReasonFinal || out.Steps != 0 || len(fp.Requests) != 2 {
		t.Fatalf("out %+v requests %d", out, len(fp.Requests))
	}
	msgs := fp.Requests[1].Messages
	note := msgs[len(msgs)-1]
	if note.Role != provider.RoleUser || !strings.Contains(note.Text, `"ls"`) {
		t.Fatalf("note %+v", note)
	}
	for _, def := range fp.Requests[1].Tools {
		if !strings.Contains(note.Text, def.Name) {
			t.Fatalf("note %q misses tool %s", note.Text, def.Name)
		}
	}
	if got := noteTexts(t, h); len(got) != 1 || got[0] != note.Text {
		t.Fatalf("journal notes %q", got)
	}
	if len(starts) != 1 || starts[0].Name != "ls" || len(results) != 1 || results[0].CallID != starts[0].ID {
		t.Fatalf("starts %+v results %+v", starts, results)
	}
	want := "no tool named ls; asked the model to use a real one (1/2)"
	if !results[0].IsError || results[0].Text != want || retried {
		t.Fatalf("result %+v retried %v", results[0], retried)
	}
	for _, rec := range journalTypes(t, h) {
		if rec == journal.TypeToolStarted {
			t.Fatal("display only step was journaled as a tool call")
		}
	}
}

func TestUnknownToolWithoutNameShowsGenericLine(t *testing.T) {
	h := host.NewMemory()
	fp := &fake.Provider{Turns: []fake.Turn{unknownToolTurn(""), fake.TextTurn("done")}}
	var results []provider.ToolResult
	obs := Observer{OnToolResult: func(r provider.ToolResult) { results = append(results, r) }}
	if _, err := Run(context.Background(), unknownToolDeps(h, fp, obs), unknownToolCfg()); err != nil {
		t.Fatal(err)
	}
	want := "no such tool; asked the model to use a real one (1/2)"
	if len(results) != 1 || results[0].Text != want {
		t.Fatalf("results %+v", results)
	}
}

func TestUnknownToolBound(t *testing.T) {
	h := host.NewMemory()
	fp := &fake.Provider{Turns: []fake.Turn{
		unknownToolTurn("ls"), unknownToolTurn("ls"), unknownToolTurn("ls"), fake.TextTurn("never"),
	}}
	_, err := Run(context.Background(), unknownToolDeps(h, fp, Observer{}), unknownToolCfg())
	var ut *provider.UnknownToolError
	if !errors.As(err, &ut) || !strings.Contains(err.Error(), "gave up after 2 corrections") {
		t.Fatalf("err %v", err)
	}
	if len(fp.Requests) != maxToolNotes+1 || len(noteTexts(t, h)) != maxToolNotes {
		t.Fatalf("requests %d notes %d", len(fp.Requests), len(noteTexts(t, h)))
	}
}

func TestUnknownToolBoundResetsAfterSuccessfulStep(t *testing.T) {
	h := host.NewMemory()
	fp := &fake.Provider{Turns: []fake.Turn{
		unknownToolTurn("ls"), unknownToolTurn("ls"),
		fake.ToolTurn(readCall("c1")),
		unknownToolTurn("ls"), unknownToolTurn("ls"),
		fake.TextTurn("done"),
	}}
	out, err := Run(context.Background(), unknownToolDeps(h, fp, Observer{}), unknownToolCfg())
	if err != nil {
		t.Fatal(err)
	}
	if out.Text != "done" || len(noteTexts(t, h)) != 4 {
		t.Fatalf("out %+v notes %d", out, len(noteTexts(t, h)))
	}
}

func TestResumeAfterCrashDuringUnknownToolRetry(t *testing.T) {
	h := host.NewMemory()
	crash := errors.New("killed")
	fpRun := &fake.Provider{Turns: []fake.Turn{unknownToolTurn("ls"), {Err: crash}}}
	if _, err := Run(context.Background(), unknownToolDeps(h, fpRun, Observer{}), unknownToolCfg()); !errors.Is(err, crash) {
		t.Fatalf("err %v", err)
	}
	notes := noteTexts(t, h)
	if len(notes) != 1 {
		t.Fatalf("notes %q", notes)
	}

	fpResume := &fake.Provider{Turns: []fake.Turn{unknownToolTurn("ls"), fake.TextTurn("done")}}
	out, err := Resume(context.Background(), unknownToolDeps(h, fpResume, Observer{}), "/j")
	if err != nil {
		t.Fatal(err)
	}
	first := fpResume.Requests[0].Messages
	if out.Text != "done" || first[len(first)-1].Text != notes[0] {
		t.Fatalf("out %+v last %+v", out, first[len(first)-1])
	}
	if len(noteTexts(t, h)) != 2 {
		t.Fatalf("notes %q", noteTexts(t, h))
	}
}

func TestResumeKeepsUnknownToolBound(t *testing.T) {
	h := host.NewMemory()
	crash := errors.New("killed")
	fpRun := &fake.Provider{Turns: []fake.Turn{unknownToolTurn("ls"), unknownToolTurn("ls"), {Err: crash}}}
	if _, err := Run(context.Background(), unknownToolDeps(h, fpRun, Observer{}), unknownToolCfg()); !errors.Is(err, crash) {
		t.Fatalf("err %v", err)
	}
	fpResume := &fake.Provider{Turns: []fake.Turn{unknownToolTurn("ls"), fake.TextTurn("never")}}
	_, err := Resume(context.Background(), unknownToolDeps(h, fpResume, Observer{}), "/j")
	var ut *provider.UnknownToolError
	if !errors.As(err, &ut) || len(fpResume.Requests) != 1 || len(noteTexts(t, h)) != maxToolNotes {
		t.Fatalf("err %v requests %d notes %d", err, len(fpResume.Requests), len(noteTexts(t, h)))
	}
}
