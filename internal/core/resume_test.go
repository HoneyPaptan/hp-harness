package core

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/HoneyPaptan/hp-harness/internal/host"
	"github.com/HoneyPaptan/hp-harness/internal/journal"
	"github.com/HoneyPaptan/hp-harness/internal/provider"
	"github.com/HoneyPaptan/hp-harness/internal/provider/fake"
	"github.com/HoneyPaptan/hp-harness/internal/tool"
)

func TestResumeAfterToolStartedCrash(t *testing.T) {
	h := host.NewMemory()
	var runs atomic.Int32
	crash := tool.Tool{
		Name: "crash", Schema: json.RawMessage(`{"type":"object"}`),
		Run: func(ctx context.Context, env tool.Env, raw json.RawMessage) (tool.Result, error) {
			runs.Add(1)
			return tool.Result{}, errors.New("stopped")
		},
	}
	reg := tool.NewRegistry(append(tool.Builtins(), crash)...)
	call := provider.ToolCall{ID: "c1", Name: "crash", Args: json.RawMessage(`{}`)}
	fpRun := &fake.Provider{Turns: []fake.Turn{fake.ToolTurn(call)}}
	cfg := Config{
		RunID: "r", Model: "m", System: "s", Dir: "/w",
		JournalPath: "/j", Prompt: "go", StepLimit: 5,
	}
	if _, err := Run(context.Background(), Deps{Host: h, Provider: fpRun, Tools: reg}, cfg); err == nil {
		t.Fatal("expected error")
	}
	fpResume := &fake.Provider{Turns: []fake.Turn{fake.TextTurn("done")}}
	out, err := Resume(context.Background(), Deps{Host: h, Provider: fpResume, Tools: reg}, "/j")
	if err != nil {
		t.Fatal(err)
	}
	if out.Text != "done" || out.Reason != ReasonFinal {
		t.Fatalf("out %+v", out)
	}
	if runs.Load() != 1 {
		t.Fatalf("runs %d", runs.Load())
	}
	res := fpResume.Requests[0].Messages[len(fpResume.Requests[0].Messages)-1].ToolResults[0]
	if !res.IsError || res.Text != interruptedMsg {
		t.Fatalf("res %+v", res)
	}
}

func TestResumeFinishedUnchanged(t *testing.T) {
	h := host.NewMemory()
	reg := tool.NewRegistry(tool.Builtins()...)
	fp := &fake.Provider{Turns: []fake.Turn{fake.TextTurn("hi")}}
	cfg := Config{
		RunID: "r", Model: "m", System: "s", Dir: "/w",
		JournalPath: "/j", Prompt: "p", StepLimit: 5,
	}
	if _, err := Run(context.Background(), Deps{Host: h, Provider: fp, Tools: reg}, cfg); err != nil {
		t.Fatal(err)
	}
	before, err := h.ReadFile("/j")
	if err != nil {
		t.Fatal(err)
	}
	_, err = Resume(context.Background(), Deps{Host: h, Provider: fp, Tools: reg}, "/j")
	if !errors.Is(err, ErrFinished) {
		t.Fatalf("err %v", err)
	}
	after, err := h.ReadFile("/j")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("journal changed")
	}
}

func TestResumeRunsUnstartedTools(t *testing.T) {
	h := host.NewMemory()
	h.WriteFile("/w/a", []byte("data"))
	reg := tool.NewRegistry(tool.Builtins()...)
	start := journal.RunStart{RunID: "r", Model: "m", System: "s", Dir: "/w", StepLimit: 5}
	w, err := journal.Create(h, "/j", start)
	if err != nil {
		t.Fatal(err)
	}
	call := provider.ToolCall{ID: "1", Name: "read", Args: json.RawMessage(`{"path":"a"}`)}
	if err := w.Assistant(provider.Message{Role: provider.RoleAssistant, ToolCalls: []provider.ToolCall{call}}); err != nil {
		t.Fatal(err)
	}
	fp := &fake.Provider{Turns: []fake.Turn{fake.TextTurn("ok")}}
	out, err := Resume(context.Background(), Deps{Host: h, Provider: fp, Tools: reg}, "/j")
	if err != nil {
		t.Fatal(err)
	}
	if out.Text != "ok" {
		t.Fatal(out)
	}
	recs, _ := journal.Read(h, "/j")
	found := false
	for _, rec := range recs {
		if rec.Type == journal.TypeToolResult {
			found = true
		}
	}
	if !found {
		t.Fatal("no tool result")
	}
}

func TestResumeSeqIncreasing(t *testing.T) {
	h := host.NewMemory()
	reg := tool.NewRegistry(tool.Builtins()...)
	crash := tool.Tool{
		Name: "crash", Schema: json.RawMessage(`{"type":"object"}`),
		Run: func(ctx context.Context, env tool.Env, raw json.RawMessage) (tool.Result, error) {
			return tool.Result{}, errors.New("x")
		},
	}
	reg = tool.NewRegistry(append(tool.Builtins(), crash)...)
	call := provider.ToolCall{ID: "1", Name: "crash", Args: json.RawMessage(`{}`)}
	fp := &fake.Provider{Turns: []fake.Turn{fake.ToolTurn(call)}}
	cfg := Config{
		RunID: "r", Model: "m", System: "s", Dir: "/w",
		JournalPath: "/j", Prompt: "p", StepLimit: 5,
	}
	if _, err := Run(context.Background(), Deps{Host: h, Provider: fp, Tools: reg}, cfg); err == nil {
		t.Fatal()
	}
	before, _ := journal.Read(h, "/j")
	last := before[len(before)-1].Seq
	fp2 := &fake.Provider{Turns: []fake.Turn{fake.TextTurn("y")}}
	if _, err := Resume(context.Background(), Deps{Host: h, Provider: fp2, Tools: reg}, "/j"); err != nil {
		t.Fatal(err)
	}
	after, _ := journal.Read(h, "/j")
	for i := 1; i < len(after); i++ {
		if after[i].Seq <= after[i-1].Seq {
			t.Fatalf("seq order at %d", i)
		}
	}
	if after[len(after)-1].Seq <= last {
		t.Fatalf("last seq %d prev %d", after[len(after)-1].Seq, last)
	}
}

func TestResumeMaxTokens(t *testing.T) {
	h := host.NewMemory()
	reg := tool.NewRegistry(tool.Builtins()...)
	start := journal.RunStart{
		RunID: "r", Model: "m", System: "s", Dir: "/w", StepLimit: 5, MaxTokens: 1234,
	}
	if _, err := journal.Create(h, "/j", start); err != nil {
		t.Fatal(err)
	}
	w, _, err := journal.Open(h, "/j")
	if err != nil {
		t.Fatal(err)
	}
	if err := w.User("u"); err != nil {
		t.Fatal(err)
	}
	fp := &fake.Provider{Turns: []fake.Turn{fake.TextTurn("ok")}}
	if _, err := Resume(context.Background(), Deps{Host: h, Provider: fp, Tools: reg}, "/j"); err != nil {
		t.Fatal(err)
	}
	if len(fp.Requests) != 1 || fp.Requests[0].MaxTokens != 1234 {
		t.Fatalf("reqs %+v", fp.Requests)
	}
}

func TestResumeCrashBeforeRunEnd(t *testing.T) {
	h := host.NewMemory()
	start := journal.RunStart{RunID: "r", Model: "m", System: "s", Dir: "/w", StepLimit: 5}
	w, err := journal.Create(h, "/j", start)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.User("u"); err != nil {
		t.Fatal(err)
	}
	if err := w.Assistant(provider.Message{Role: provider.RoleAssistant, Text: "answer"}); err != nil {
		t.Fatal(err)
	}
	reg := tool.NewRegistry(tool.Builtins()...)
	fp := &fake.Provider{}
	out, err := Resume(context.Background(), Deps{Host: h, Provider: fp, Tools: reg}, "/j")
	if err != nil {
		t.Fatal(err)
	}
	if out.Text != "answer" || out.Reason != ReasonFinal {
		t.Fatalf("out %+v", out)
	}
}
