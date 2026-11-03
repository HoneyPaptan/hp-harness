package core

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/HoneyPaptan/hp-harness/internal/host"
	"github.com/HoneyPaptan/hp-harness/internal/journal"
	"github.com/HoneyPaptan/hp-harness/internal/provider"
	"github.com/HoneyPaptan/hp-harness/internal/provider/fake"
	"github.com/HoneyPaptan/hp-harness/internal/tool"
)

func gateCfg() Config {
	return Config{
		RunID: "r", Model: "m", System: "s", Dir: "/w",
		JournalPath: "/j", Prompt: "go", StepLimit: 10,
	}
}

func countingTool(name string, runs *atomic.Int32) tool.Tool {
	return tool.Tool{
		Name: name, Schema: json.RawMessage(`{"type":"object"}`),
		Run: func(ctx context.Context, env tool.Env, raw json.RawMessage) (tool.Result, error) {
			runs.Add(1)
			return tool.Result{Text: "ok"}, nil
		},
	}
}

func TestGateNilAllowsRun(t *testing.T) {
	h := host.NewMemory()
	var runs atomic.Int32
	reg := tool.NewRegistry(countingTool("work", &runs))
	call := provider.ToolCall{ID: "1", Name: "work", Args: json.RawMessage(`{}`)}
	fp := &fake.Provider{Turns: []fake.Turn{
		fake.ToolTurn(call),
		fake.TextTurn("done"),
	}}
	out, err := Run(context.Background(), Deps{Host: h, Provider: fp, Tools: reg}, gateCfg())
	if err != nil {
		t.Fatal(err)
	}
	if out.Reason != ReasonFinal || runs.Load() != 1 {
		t.Fatalf("out %+v runs %d", out, runs.Load())
	}
}

func TestGateAllowRunsAndSeesCall(t *testing.T) {
	h := host.NewMemory()
	var runs atomic.Int32
	reg := tool.NewRegistry(countingTool("work", &runs))
	args := json.RawMessage(`{"x":1}`)
	call := provider.ToolCall{ID: "1", Name: "work", Args: args}
	var seen provider.ToolCall
	gate := func(ctx context.Context, c provider.ToolCall) Verdict {
		seen = c
		return Verdict{Allow: true}
	}
	fp := &fake.Provider{Turns: []fake.Turn{
		fake.ToolTurn(call),
		fake.TextTurn("done"),
	}}
	d := Deps{Host: h, Provider: fp, Tools: reg, Gate: gate}
	if _, err := Run(context.Background(), d, gateCfg()); err != nil {
		t.Fatal(err)
	}
	if runs.Load() != 1 {
		t.Fatalf("runs %d", runs.Load())
	}
	if seen.Name != call.Name || !bytes.Equal(seen.Args, args) {
		t.Fatalf("seen %+v", seen)
	}
}

func TestGateHoldWithReason(t *testing.T) {
	h := host.NewMemory()
	var runs atomic.Int32
	reg := tool.NewRegistry(countingTool("bash", &runs))
	call := provider.ToolCall{ID: "1", Name: "bash", Args: json.RawMessage(`{"cmd":"git push"}`)}
	reason := "denied by rule bash git push *"
	gate := func(ctx context.Context, c provider.ToolCall) Verdict {
		return Verdict{Allow: false, Reason: reason}
	}
	fp := &fake.Provider{Turns: []fake.Turn{
		fake.ToolTurn(call),
		fake.TextTurn("done"),
	}}
	d := Deps{Host: h, Provider: fp, Tools: reg, Gate: gate}
	out, err := Run(context.Background(), d, gateCfg())
	if err != nil {
		t.Fatal(err)
	}
	if out.Text != "done" || out.Reason != ReasonFinal {
		t.Fatalf("out %+v", out)
	}
	if runs.Load() != 0 {
		t.Fatalf("runs %d", runs.Load())
	}
	want := "held: " + reason
	recs, err := journal.Read(h, "/j")
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, rec := range recs {
		if rec.Type != journal.TypeToolResult {
			continue
		}
		var td journal.ToolResultData
		if err := json.Unmarshal(rec.Data, &td); err != nil {
			t.Fatal(err)
		}
		tr := td.Result
		if tr.Text == want && tr.IsError {
			found = true
		}
	}
	if !found {
		t.Fatal("no held tool result in journal")
	}
	last := fp.Requests[len(fp.Requests)-1].Messages[len(fp.Requests[len(fp.Requests)-1].Messages)-1]
	if len(last.ToolResults) != 1 || last.ToolResults[0].Text != want || !last.ToolResults[0].IsError {
		t.Fatalf("results %+v", last.ToolResults)
	}
}

func TestGateHoldEmptyReason(t *testing.T) {
	h := host.NewMemory()
	var runs atomic.Int32
	reg := tool.NewRegistry(countingTool("work", &runs))
	call := provider.ToolCall{ID: "1", Name: "work", Args: json.RawMessage(`{}`)}
	gate := func(ctx context.Context, c provider.ToolCall) Verdict {
		return Verdict{Allow: false}
	}
	fp := &fake.Provider{Turns: []fake.Turn{
		fake.ToolTurn(call),
		fake.TextTurn("x"),
	}}
	d := Deps{Host: h, Provider: fp, Tools: reg, Gate: gate}
	if _, err := Run(context.Background(), d, gateCfg()); err != nil {
		t.Fatal(err)
	}
	if runs.Load() != 0 {
		t.Fatalf("runs %d", runs.Load())
	}
	last := fp.Requests[len(fp.Requests)-1].Messages[len(fp.Requests[len(fp.Requests)-1].Messages)-1]
	if len(last.ToolResults) != 1 || last.ToolResults[0].Text != "held: not allowed" {
		t.Fatalf("results %+v", last.ToolResults)
	}
}

func TestGateBlocksUntilContextCanceled(t *testing.T) {
	h := host.NewMemory()
	var runs atomic.Int32
	reg := tool.NewRegistry(countingTool("work", &runs))
	call := provider.ToolCall{ID: "1", Name: "work", Args: json.RawMessage(`{}`)}
	ctx, cancel := context.WithCancel(context.Background())
	gate := func(ctx context.Context, c provider.ToolCall) Verdict {
		<-ctx.Done()
		return Verdict{Allow: true}
	}
	fp := &fake.Provider{Turns: []fake.Turn{fake.ToolTurn(call)}}
	d := Deps{Host: h, Provider: fp, Tools: reg, Gate: gate}
	done := make(chan error, 1)
	go func() {
		_, err := Run(ctx, d, gateCfg())
		done <- err
	}()
	cancel()
	err := <-done
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err %v", err)
	}
	if runs.Load() != 0 {
		t.Fatalf("runs %d", runs.Load())
	}
}

func TestResumeStartedToolSkipsGate(t *testing.T) {
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
	cfg := gateCfg()
	if _, err := Run(context.Background(), Deps{Host: h, Provider: fpRun, Tools: reg}, cfg); err == nil {
		t.Fatal("expected error")
	}
	if runs.Load() != 1 {
		t.Fatalf("runs %d", runs.Load())
	}
	var gated atomic.Int32
	hold := func(ctx context.Context, c provider.ToolCall) Verdict {
		gated.Add(1)
		return Verdict{Allow: false, Reason: "policy wait"}
	}
	fpResume := &fake.Provider{Turns: []fake.Turn{fake.TextTurn("done")}}
	d := Deps{Host: h, Provider: fpResume, Tools: reg, Gate: hold}
	out, err := Resume(context.Background(), d, "/j")
	if err != nil {
		t.Fatal(err)
	}
	if out.Text != "done" || runs.Load() != 1 || gated.Load() != 0 {
		t.Fatalf("out %+v runs %d gated %d", out, runs.Load(), gated.Load())
	}
	res := fpResume.Requests[0].Messages[len(fpResume.Requests[0].Messages)-1].ToolResults[0]
	if !res.IsError || strings.HasPrefix(res.Text, "held:") {
		t.Fatalf("res %+v", res)
	}
}
