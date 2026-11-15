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

func TestUnknownToolRecovers(t *testing.T) {
	h := host.NewMemory()
	fp := &fake.Provider{Turns: []fake.Turn{unknownToolTurn("ls"), fake.TextTurn("done")}}
	var retries [][2]int
	obs := Observer{OnRetry: func(_ time.Duration, attempt, max int) { retries = append(retries, [2]int{attempt, max}) }}
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
	if len(retries) != 1 || retries[0] != [2]int{1, maxToolNotes} {
		t.Fatalf("retries %v", retries)
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
