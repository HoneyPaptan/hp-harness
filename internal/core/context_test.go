package core

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/HoneyPaptan/hp-harness/internal/host"
	"github.com/HoneyPaptan/hp-harness/internal/provider"
	"github.com/HoneyPaptan/hp-harness/internal/provider/fake"
	"github.com/HoneyPaptan/hp-harness/internal/tool"
)

func TestOutcomeContextTokensIsLastCallNotSum(t *testing.T) {
	h := host.NewMemory()
	h.WriteFile("/w/f.txt", []byte("hello"))
	call := provider.ToolCall{ID: "1", Name: "read", Args: json.RawMessage(`{"path":"f.txt"}`)}
	fp := &fake.Provider{Turns: []fake.Turn{
		{Events: []provider.Event{
			{Kind: provider.EventToolCall, Call: call},
			{Kind: provider.EventDone, StopReason: "tool_use", Usage: provider.Usage{InputTokens: 300, OutputTokens: 20}},
		}},
		{Events: []provider.Event{
			{Kind: provider.EventText, Text: "done"},
			{Kind: provider.EventDone, StopReason: "end_turn", Usage: provider.Usage{InputTokens: 400, OutputTokens: 30}},
		}},
	}}
	cfg := Config{RunID: "r", Model: "m", System: "s", Dir: "/w", JournalPath: "/j", Prompt: "go", StepLimit: 10}
	out, err := Run(context.Background(), Deps{Host: h, Provider: fp, Tools: tool.NewRegistry(tool.Builtins()...)}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if out.Usage.InputTokens != 700 {
		t.Fatalf("summed input %d", out.Usage.InputTokens)
	}
	if out.ContextTokens != 430 {
		t.Fatalf("context tokens %d, want the last call's 430", out.ContextTokens)
	}
}
