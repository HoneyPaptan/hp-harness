package core

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/HoneyPaptan/hp-harness/internal/host"
	"github.com/HoneyPaptan/hp-harness/internal/provider"
	"github.com/HoneyPaptan/hp-harness/internal/provider/fake"
	"github.com/HoneyPaptan/hp-harness/internal/tool"
)

func TestSessionIDIsTheRunIDAcrossResume(t *testing.T) {
	h := host.NewMemory()
	crash := tool.Tool{
		Name: "crash", Schema: json.RawMessage(`{"type":"object"}`),
		Run: func(ctx context.Context, env tool.Env, raw json.RawMessage) (tool.Result, error) {
			return tool.Result{}, errors.New("stopped")
		},
	}
	reg := tool.NewRegistry(append(tool.Builtins(), crash)...)
	call := provider.ToolCall{ID: "c1", Name: "crash", Args: json.RawMessage(`{}`)}
	first := &fake.Provider{Turns: []fake.Turn{fake.ToolTurn(call)}}
	cfg := Config{
		RunID: "run-7", Model: "m", System: "s", Dir: "/w",
		JournalPath: "/j", Prompt: "go", StepLimit: 5,
	}
	if _, err := Run(context.Background(), Deps{Host: h, Provider: first, Tools: reg}, cfg); err == nil {
		t.Fatal("expected error")
	}
	resumed := &fake.Provider{Turns: []fake.Turn{fake.TextTurn("done")}}
	if _, err := Resume(context.Background(), Deps{Host: h, Provider: resumed, Tools: reg}, "/j"); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		reqs []provider.Request
	}{
		{"fresh run", first.Requests},
		{"resumed run", resumed.Requests},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if len(tc.reqs) == 0 {
				t.Fatal("no requests")
			}
			for _, r := range tc.reqs {
				if r.SessionID != "run-7" {
					t.Fatalf("SessionID = %q", r.SessionID)
				}
			}
		})
	}
}
