package core

import (
	"context"
	"encoding/json"
	"slices"
	"testing"

	"github.com/HoneyPaptan/hp-harness/internal/host"
	"github.com/HoneyPaptan/hp-harness/internal/provider"
	"github.com/HoneyPaptan/hp-harness/internal/provider/fake"
	"github.com/HoneyPaptan/hp-harness/internal/tool"
)

func TestExecEnvReachesTool(t *testing.T) {
	h := host.NewMemory()
	h.ExecFunc = func(context.Context, host.ExecRequest) (host.ExecResult, error) {
		return host.ExecResult{ExitCode: 0}, nil
	}
	reg := tool.NewRegistry(tool.Builtins()...)
	call := provider.ToolCall{ID: "1", Name: "bash", Args: json.RawMessage(`{"command":"true"}`)}
	fp := &fake.Provider{Turns: []fake.Turn{
		fake.ToolTurn(call),
		fake.TextTurn("ok"),
	}}
	want := []string{"FROM_DEPS=1"}
	cfg := Config{
		RunID: "r", Model: "m", System: "s", Dir: "/w",
		JournalPath: "/j", Prompt: "go", StepLimit: 5,
	}
	_, err := Run(context.Background(), Deps{
		Host: h, Provider: fp, Tools: reg,
		ExecEnv: func() []string { return append([]string(nil), want...) },
	}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(h.LastExec.Env, want) {
		t.Fatalf("env %v", h.LastExec.Env)
	}
}
