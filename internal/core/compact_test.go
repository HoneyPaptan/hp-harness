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

func win1000(string) int { return 1000 }

func fatToolTurn(call provider.ToolCall, usage int) fake.Turn {
	t := fake.Turn{Events: []provider.Event{
		{Kind: provider.EventText, Text: strings.Repeat("y", 800)},
		{Kind: provider.EventToolCall, Call: call},
		{Kind: provider.EventDone, StopReason: "tool_use", Usage: provider.Usage{InputTokens: usage, OutputTokens: 0}},
	}}
	return t
}

func compactDeps(h *host.Memory, fp *fake.Provider) Deps {
	return Deps{
		Host: h, Provider: fp, Tools: tool.NewRegistry(tool.Builtins()...),
		ContextWindow: win1000,
	}
}

func compactCfg(h *host.Memory, prompt string) Config {
	if prompt == "" {
		prompt = strings.Repeat("q", 400)
	}
	return Config{
		RunID: "r", Model: "m", System: "s", Dir: "/w",
		JournalPath: "/j", Prompt: prompt, StepLimit: 5,
	}
}

func TestSplitKeepTable(t *testing.T) {
	big := strings.Repeat("x", 400)
	msgs := []provider.Message{
		{Role: provider.RoleUser, Text: "old"},
		{Role: provider.RoleAssistant, Text: big},
		{Role: provider.RoleUser, Text: "tail"},
	}
	if got := splitKeep(msgs, 1000); got != 2 {
		t.Fatalf("plain keep %d", got)
	}
	call := provider.ToolCall{ID: "1", Name: "read"}
	step := []provider.Message{
		{Role: provider.RoleUser, Text: "u"},
		{Role: provider.RoleAssistant, Text: big, ToolCalls: []provider.ToolCall{call}},
		{Role: provider.RoleUser, ToolResults: []provider.ToolResult{{CallID: "1", Text: "r"}}},
		{Role: provider.RoleUser, Text: "tail"},
	}
	if got := splitKeep(step, 1000); got != 3 {
		t.Fatalf("tool step keep %d", got)
	}
	trOnly := []provider.Message{
		{Role: provider.RoleUser, Text: "a"},
		{Role: provider.RoleAssistant, Text: big},
		{Role: provider.RoleUser, ToolResults: []provider.ToolResult{{CallID: "1", Text: "r"}}},
	}
	if got := splitKeep(trOnly, 1000); got != 2 {
		t.Fatalf("tail tool result keep %d", got)
	}
	one := []provider.Message{{Role: provider.RoleUser, Text: "only"}}
	if got := splitKeep(one, 1000); got != 0 {
		t.Fatalf("single keep %d", got)
	}
}

func TestAutoCompactAfterThreshold(t *testing.T) {
	h := host.NewMemory()
	h.WriteFile("/w/f.txt", []byte("x"))
	call := provider.ToolCall{ID: "1", Name: "read", Args: json.RawMessage(`{"path":"f.txt"}`)}
	t1 := fatToolTurn(call, 100)
	t2 := fatToolTurn(call, 900)
	fp := &fake.Provider{Turns: []fake.Turn{
		t1,
		t2,
		fake.TextTurn("summary note"),
		fake.TextTurn("done"),
	}}
	var kept int
	deps := compactDeps(h, fp)
	deps.Observer = Observer{OnCompact: func(k int) { kept = k }}
	cfg := compactCfg(h, strings.Repeat("q", 100))
	out, err := Run(context.Background(), deps, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if out.Text != "done" || kept == 0 {
		t.Fatalf("out %+v kept %d", out, kept)
	}
	if len(fp.Requests) != 4 {
		t.Fatalf("requests %d", len(fp.Requests))
	}
	sum := fp.Requests[2]
	if len(sum.Tools) != 0 || sum.System != summarySystemPrompt {
		t.Fatalf("summary req tools %d system %q", len(sum.Tools), sum.System)
	}
	req2 := fp.Requests[3]
	if !strings.HasPrefix(req2.Messages[0].Text, handoffPrefix) {
		t.Fatalf("handoff missing")
	}
	if !strings.Contains(req2.Messages[0].Text, strings.Repeat("q", 100)) {
		t.Fatalf("prompt missing in handoff")
	}
	recs, _ := journal.Read(h, "/j")
	var compact bool
	for _, rec := range recs {
		if rec.Type == journal.TypeCompact {
			compact = true
		}
	}
	if !compact {
		t.Fatal("no compact record")
	}
}

func TestNoCompactBelowThreshold(t *testing.T) {
	h := host.NewMemory()
	h.WriteFile("/w/f.txt", []byte("x"))
	call := provider.ToolCall{ID: "1", Name: "read", Args: json.RawMessage(`{"path":"f.txt"}`)}
	t1 := fatToolTurn(call, 100)
	fp := &fake.Provider{Turns: []fake.Turn{
		t1,
		fake.TextTurn("b"),
	}}
	deps := compactDeps(h, fp)
	if _, err := Run(context.Background(), deps, compactCfg(h, "")); err != nil {
		t.Fatal(err)
	}
	if len(fp.Requests) != 2 {
		t.Fatalf("requests %d", len(fp.Requests))
	}
}

func TestSecondCompactionHandoffInSummary(t *testing.T) {
	h := host.NewMemory()
	h.WriteFile("/w/f.txt", []byte("x"))
	call := provider.ToolCall{ID: "1", Name: "read", Args: json.RawMessage(`{"path":"f.txt"}`)}
	mk := func(u int) fake.Turn { return fatToolTurn(call, u) }
	fp := &fake.Provider{Turns: []fake.Turn{
		mk(100),
		mk(900),
		fake.TextTurn("sum1"),
		mk(900),
		fake.TextTurn("sum2"),
		fake.TextTurn("c"),
	}}
	deps := compactDeps(h, fp)
	if _, err := Run(context.Background(), deps, compactCfg(h, "")); err != nil {
		t.Fatal(err)
	}
	var sumReqs []provider.Request
	for _, req := range fp.Requests {
		if req.System == summarySystemPrompt {
			sumReqs = append(sumReqs, req)
		}
	}
	if len(sumReqs) < 2 {
		t.Fatalf("summary requests %d", len(sumReqs))
	}
	body := sumReqs[1].Messages[0].Text
	if !strings.Contains(body, handoffPrefix) {
		t.Fatalf("summary input missing prior handoff")
	}
}

func TestOverflowCompactOnce(t *testing.T) {
	h := host.NewMemory()
	h.WriteFile("/w/f.txt", []byte("x"))
	call := provider.ToolCall{ID: "1", Name: "read", Args: json.RawMessage(`{"path":"f.txt"}`)}
	fp := &fake.Provider{Turns: []fake.Turn{
		fatToolTurn(call, 0),
		{Err: provider.ErrContextOverflow},
		fake.TextTurn("sum"),
		fake.TextTurn("ok"),
	}}
	deps := compactDeps(h, fp)
	out, err := Run(context.Background(), deps, compactCfg(h, strings.Repeat("q", 400)))
	if err != nil {
		t.Fatal(err)
	}
	if out.Text != "ok" || len(fp.Requests) != 4 {
		t.Fatalf("out %+v reqs %d", out, len(fp.Requests))
	}
}

func TestOverflowTwiceFails(t *testing.T) {
	h := host.NewMemory()
	h.WriteFile("/w/f.txt", []byte("x"))
	call := provider.ToolCall{ID: "1", Name: "read", Args: json.RawMessage(`{"path":"f.txt"}`)}
	fp := &fake.Provider{Turns: []fake.Turn{
		fatToolTurn(call, 0),
		{Err: provider.ErrContextOverflow},
		fake.TextTurn("sum"),
		{Err: provider.ErrContextOverflow},
	}}
	deps := compactDeps(h, fp)
	_, err := Run(context.Background(), deps, compactCfg(h, strings.Repeat("q", 400)))
	if !errors.Is(err, provider.ErrContextOverflow) {
		t.Fatalf("err %v", err)
	}
}

func TestSummaryFailureKeepsMessages(t *testing.T) {
	h := host.NewMemory()
	h.WriteFile("/w/f.txt", []byte("x"))
	call := provider.ToolCall{ID: "1", Name: "read", Args: json.RawMessage(`{"path":"f.txt"}`)}
	t1 := fatToolTurn(call, 100)
	t2 := fatToolTurn(call, 900)
	fp := &fake.Provider{Turns: []fake.Turn{
		t1,
		t2,
		fake.TextTurn(""),
		fake.TextTurn("ok"),
	}}
	deps := compactDeps(h, fp)
	if _, err := Run(context.Background(), deps, compactCfg(h, "")); err != nil {
		t.Fatal(err)
	}
	req := fp.Requests[2]
	if strings.HasPrefix(req.Messages[0].Text, handoffPrefix) {
		t.Fatal("unexpected compact after summary failure")
	}
}

func TestResumeAfterCompact(t *testing.T) {
	h := host.NewMemory()
	h.WriteFile("/w/f.txt", []byte("x"))
	call := provider.ToolCall{ID: "1", Name: "read", Args: json.RawMessage(`{"path":"f.txt"}`)}
	t1 := fatToolTurn(call, 900)
	fp := &fake.Provider{Turns: []fake.Turn{
		t1,
		fake.TextTurn("sum"),
		fatToolTurn(call, 0),
		{Err: errCut},
	}}
	deps := compactDeps(h, fp)
	if _, err := Run(context.Background(), deps, compactCfg(h, "")); !errors.Is(err, errCut) {
		t.Fatalf("err %v", err)
	}
	fp2 := &fake.Provider{Turns: []fake.Turn{fake.TextTurn("done")}}
	deps.Provider = fp2
	out, err := Resume(context.Background(), deps, "/j")
	if err != nil {
		t.Fatal(err)
	}
	if out.Text != "done" {
		t.Fatalf("out %+v", out)
	}
	req := fp2.Requests[0]
	if !strings.HasPrefix(req.Messages[0].Text, handoffPrefix) {
		t.Fatalf("resume handoff missing")
	}
}

func TestManualCompactShort(t *testing.T) {
	h := host.NewMemory()
	w, err := journal.Create(h, "/j", journal.RunStart{RunID: "r", Model: "m", System: "s", Dir: "/w", StepLimit: 5})
	if err != nil {
		t.Fatal(err)
	}
	if err := w.User("hi"); err != nil {
		t.Fatal(err)
	}
	deps := compactDeps(h, &fake.Provider{})
	ok, err := Compact(context.Background(), deps, "/j")
	if err != nil || ok {
		t.Fatalf("ok %v err %v", ok, err)
	}
}

func TestNilContextWindowNeverCompacts(t *testing.T) {
	h := host.NewMemory()
	h.WriteFile("/w/f.txt", []byte("x"))
	call := provider.ToolCall{ID: "1", Name: "read", Args: json.RawMessage(`{"path":"f.txt"}`)}
	t1 := fatToolTurn(call, 900)
	fp := &fake.Provider{Turns: []fake.Turn{
		t1,
		fake.TextTurn("b"),
	}}
	deps := Deps{Host: h, Provider: fp, Tools: tool.NewRegistry(tool.Builtins()...)}
	if _, err := Run(context.Background(), deps, compactCfg(h, "")); err != nil {
		t.Fatal(err)
	}
	if len(fp.Requests) != 2 {
		t.Fatalf("requests %d", len(fp.Requests))
	}
}

func TestUserPromptsVerbatim(t *testing.T) {
	got := fitUserPrompts([]provider.Message{
		{Role: provider.RoleUser, Text: "alpha"},
		{Role: provider.RoleUser, Text: "beta"},
	}, 100)
	if got != "alpha\nbeta" {
		t.Fatalf("got %q", got)
	}
	if fitUserPrompts([]provider.Message{
		{Role: provider.RoleUser, Text: handoffPrefix + "x"},
		{Role: provider.RoleUser, Text: "real"},
	}, 100) != "real" {
		t.Fatal()
	}
}
