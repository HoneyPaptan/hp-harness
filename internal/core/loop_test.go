package core

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/HoneyPaptan/hp-harness/internal/host"
	"github.com/HoneyPaptan/hp-harness/internal/journal"
	"github.com/HoneyPaptan/hp-harness/internal/provider"
	"github.com/HoneyPaptan/hp-harness/internal/provider/fake"
	"github.com/HoneyPaptan/hp-harness/internal/tool"
)

func TestOneToolThenAnswer(t *testing.T) {
	h := host.NewMemory()
	h.WriteFile("/w/f.txt", []byte("hello"))
	reg := tool.NewRegistry(tool.Builtins()...)
	readCall := provider.ToolCall{ID: "1", Name: "read", Args: json.RawMessage(`{"path":"f.txt"}`)}
	fp := &fake.Provider{Turns: []fake.Turn{
		fake.ToolTurn(readCall),
		fake.TextTurn("done"),
	}}
	cfg := Config{
		RunID: "r", Model: "m", System: "s", Dir: "/w",
		JournalPath: "/j", Prompt: "go", StepLimit: 10,
	}
	out, err := Run(context.Background(), Deps{Host: h, Provider: fp, Tools: reg}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if out.Text != "done" || out.Reason != ReasonFinal || out.Steps != 1 {
		t.Fatalf("out %+v", out)
	}
	req := fp.Requests[1]
	last := req.Messages[len(req.Messages)-1]
	if len(last.ToolResults) != 1 || last.ToolResults[0].Text == "" {
		t.Fatalf("results %+v", last.ToolResults)
	}
}

func TestStepLimit(t *testing.T) {
	h := host.NewMemory()
	reg := tool.NewRegistry(tool.Builtins()...)
	call := provider.ToolCall{ID: "x", Name: "read", Args: json.RawMessage(`{"path":"nope"}`)}
	turns := make([]fake.Turn, 4)
	for i := range turns {
		turns[i] = fake.ToolTurn(call)
	}
	fp := &fake.Provider{Turns: turns}
	cfg := Config{
		RunID: "r", Model: "m", System: "s", Dir: "/w",
		JournalPath: "/j", Prompt: "go", StepLimit: 3,
	}
	out, err := Run(context.Background(), Deps{Host: h, Provider: fp, Tools: reg}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if out.Reason != ReasonStepLimit || out.Steps != 3 {
		t.Fatalf("out %+v", out)
	}
	recs, err := journal.Read(h, "/j")
	if err != nil {
		t.Fatal(err)
	}
	if recs[len(recs)-1].Type != journal.TypeRunEnd {
		t.Fatalf("last type %s", recs[len(recs)-1].Type)
	}
}

func TestHistoryMatchesReplay(t *testing.T) {
	h := host.NewMemory()
	reg := tool.NewRegistry(tool.Builtins()...)
	call := provider.ToolCall{ID: "1", Name: "read", Args: json.RawMessage(`{"path":"a"}`)}
	fp := &fake.Provider{Turns: []fake.Turn{
		fake.ToolTurn(call),
		fake.TextTurn("ok"),
	}}
	cfg := Config{
		RunID: "r", Model: "m", System: "s", Dir: "/w",
		JournalPath: "/j", Prompt: "p", StepLimit: 5,
	}
	if _, err := Run(context.Background(), Deps{Host: h, Provider: fp, Tools: reg}, cfg); err != nil {
		t.Fatal(err)
	}
	recs, err := journal.Read(h, "/j")
	if err != nil {
		t.Fatal(err)
	}
	st, err := journal.Replay(recs)
	if err != nil {
		t.Fatal(err)
	}
	lastReq := fp.Requests[len(fp.Requests)-1]
	sent := st.Messages[:len(st.Messages)-1]
	if !reflect.DeepEqual(lastReq.Messages, sent) {
		t.Fatalf("history mismatch\n got %#v\nwant %#v", lastReq.Messages, sent)
	}
}

func TestObserver(t *testing.T) {
	h := host.NewMemory()
	reg := tool.NewRegistry(tool.Builtins()...)
	call := provider.ToolCall{ID: "c", Name: "read", Args: json.RawMessage(`{"path":"x"}`)}
	fp := &fake.Provider{Turns: []fake.Turn{
		fake.Turn{Events: []provider.Event{
			{Kind: provider.EventText, Text: "hi"},
			{Kind: provider.EventToolCall, Call: call},
			{Kind: provider.EventDone},
		}},
		fake.TextTurn("bye"),
	}}
	var texts []string
	var starts, results []string
	obs := Observer{
		OnEvent:      func(ev provider.Event) { texts = append(texts, ev.Text) },
		OnToolStart:  func(c provider.ToolCall) { starts = append(starts, c.ID) },
		OnToolResult: func(r provider.ToolResult) { results = append(results, r.CallID) },
	}
	cfg := Config{
		RunID: "r", Model: "m", System: "s", Dir: "/w",
		JournalPath: "/j", Prompt: "p", StepLimit: 5,
	}
	if _, err := Run(context.Background(), Deps{Host: h, Provider: fp, Tools: reg, Observer: obs}, cfg); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(texts, []string{"hi", "bye"}) {
		t.Fatalf("texts %v", texts)
	}
	if !reflect.DeepEqual(starts, []string{"c"}) || !reflect.DeepEqual(results, []string{"c"}) {
		t.Fatalf("starts %v results %v", starts, results)
	}
}

func TestObserverNilSafe(t *testing.T) {
	var o Observer
	o.emitTurnStart()
	o.emitEvent(provider.Event{})
	o.emitToolStart(provider.ToolCall{})
	o.emitToolResult(provider.ToolResult{})
	o.emitWait("x", 0, hNow())
}

func hNow() time.Time {
	return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
}

func TestOnTurnStart(t *testing.T) {
	h := host.NewMemory()
	reg := tool.NewRegistry(tool.Builtins()...)
	call := provider.ToolCall{ID: "c", Name: "read", Args: json.RawMessage(`{"path":"x"}`)}
	fp := &fake.Provider{Turns: []fake.Turn{
		fake.Turn{Events: []provider.Event{
			{Kind: provider.EventText, Text: "a"},
			{Kind: provider.EventToolCall, Call: call},
			{Kind: provider.EventDone},
		}},
		fake.TextTurn("b"),
	}}
	var order []string
	obs := Observer{
		OnTurnStart: func() { order = append(order, "start") },
		OnEvent:     func(ev provider.Event) { order = append(order, "ev:"+ev.Text) },
	}
	cfg := Config{
		RunID: "r", Model: "m", System: "s", Dir: "/w",
		JournalPath: "/j", Prompt: "p", StepLimit: 5,
	}
	if _, err := Run(context.Background(), Deps{Host: h, Provider: fp, Tools: reg, Observer: obs}, cfg); err != nil {
		t.Fatal(err)
	}
	want := []string{"start", "ev:a", "start", "ev:b"}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("order %v", order)
	}
}

func TestStepLimitDefault(t *testing.T) {
	if stepLimit(Config{}) != defaultStepLimit {
		t.Fatal()
	}
}

func TestErrFinishedType(t *testing.T) {
	if ErrFinished == nil || ErrFinished.Error() == "" {
		t.Fatal()
	}
	_ = errors.Is(ErrFinished, ErrFinished)
}
