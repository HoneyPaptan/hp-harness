package core

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/HoneyPaptan/hp-harness/internal/host"
	"github.com/HoneyPaptan/hp-harness/internal/journal"
	"github.com/HoneyPaptan/hp-harness/internal/provider"
	"github.com/HoneyPaptan/hp-harness/internal/provider/fake"
	"github.com/HoneyPaptan/hp-harness/internal/tool"
)

func TestUnknownToolContinues(t *testing.T) {
	h := host.NewMemory()
	reg := tool.NewRegistry(tool.Builtins()...)
	call := provider.ToolCall{ID: "1", Name: "deploy", Args: json.RawMessage(`{}`)}
	fp := &fake.Provider{Turns: []fake.Turn{
		fake.ToolTurn(call),
		fake.TextTurn("ok"),
	}}
	cfg := Config{
		RunID: "r", Model: "m", System: "s", Dir: "/w",
		JournalPath: "/j", Prompt: "p", StepLimit: 5,
	}
	out, err := Run(context.Background(), Deps{Host: h, Provider: fp, Tools: reg}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if out.Text != "ok" || len(fp.Requests) != 2 {
		t.Fatalf("out %+v requests %d", out, len(fp.Requests))
	}
	res := fp.Requests[1].Messages[len(fp.Requests[1].Messages)-1].ToolResults[0]
	if !res.IsError || !strings.Contains(res.Text, "unknown tool") {
		t.Fatalf("res %+v", res)
	}
}

func TestBadToolArgsContinues(t *testing.T) {
	h := host.NewMemory()
	reg := tool.NewRegistry(tool.Builtins()...)
	call := provider.ToolCall{
		ID: "1", Name: "read", Args: json.RawMessage(`{"path":"x","nope":1}`),
	}
	fp := &fake.Provider{Turns: []fake.Turn{
		fake.ToolTurn(call),
		fake.TextTurn("ok"),
	}}
	cfg := Config{
		RunID: "r", Model: "m", System: "s", Dir: "/w",
		JournalPath: "/j", Prompt: "p", StepLimit: 5,
	}
	out, err := Run(context.Background(), Deps{Host: h, Provider: fp, Tools: reg}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if out.Text != "ok" {
		t.Fatal(out)
	}
	res := fp.Requests[1].Messages[len(fp.Requests[1].Messages)-1].ToolResults[0]
	if !res.IsError {
		t.Fatalf("res %+v", res)
	}
}

func TestProviderErrorMidTurn(t *testing.T) {
	h := host.NewMemory()
	reg := tool.NewRegistry(tool.Builtins()...)
	streamErr := errors.New("stream broke")
	fp := &fake.Provider{Turns: []fake.Turn{{
		Events: []provider.Event{{Kind: provider.EventText, Text: "partial"}},
		Err:    streamErr,
	}}}
	cfg := Config{
		RunID: "r", Model: "m", System: "s", Dir: "/w",
		JournalPath: "/j", Prompt: "p", StepLimit: 5,
	}
	_, err := Run(context.Background(), Deps{Host: h, Provider: fp, Tools: reg}, cfg)
	if !errors.Is(err, streamErr) {
		t.Fatalf("err %v", err)
	}
	recs, _ := journal.Read(h, "/j")
	for _, rec := range recs {
		if rec.Type == journal.TypeAssistant || rec.Type == journal.TypeRunEnd {
			t.Fatalf("record %s", rec.Type)
		}
	}
}

func TestContextCanceledStreaming(t *testing.T) {
	h := host.NewMemory()
	reg := tool.NewRegistry(tool.Builtins()...)
	fp := &fake.Provider{Turns: []fake.Turn{{
		Events: []provider.Event{
			{Kind: provider.EventText, Text: "x"},
			{Kind: provider.EventDone},
		},
	}}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cfg := Config{
		RunID: "r", Model: "m", System: "s", Dir: "/w",
		JournalPath: "/j", Prompt: "p", StepLimit: 5,
	}
	_, err := Run(ctx, Deps{Host: h, Provider: fp, Tools: reg}, cfg)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err %v", err)
	}
	recs, _ := journal.Read(h, "/j")
	for _, rec := range recs {
		if rec.Type == journal.TypeRunEnd {
			t.Fatal("run_end")
		}
	}
}

func TestToolRunError(t *testing.T) {
	h := host.NewMemory()
	boom := errors.New("host fail")
	fail := tool.Tool{
		Name: "fail", Schema: json.RawMessage(`{"type":"object"}`),
		Run: func(ctx context.Context, env tool.Env, raw json.RawMessage) (tool.Result, error) {
			return tool.Result{}, boom
		},
	}
	reg := tool.NewRegistry(append(tool.Builtins(), fail)...)
	call := provider.ToolCall{ID: "1", Name: "fail", Args: json.RawMessage(`{}`)}
	fp := &fake.Provider{Turns: []fake.Turn{fake.ToolTurn(call)}}
	cfg := Config{
		RunID: "r", Model: "m", System: "s", Dir: "/w",
		JournalPath: "/j", Prompt: "p", StepLimit: 5,
	}
	_, err := Run(context.Background(), Deps{Host: h, Provider: fp, Tools: reg}, cfg)
	if !errors.Is(err, boom) {
		t.Fatalf("err %v", err)
	}
	recs, _ := journal.Read(h, "/j")
	if recs[len(recs)-1].Type != journal.TypeToolStarted {
		t.Fatalf("last %s", recs[len(recs)-1].Type)
	}
}
