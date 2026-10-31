package core

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sync/atomic"
	"testing"

	"github.com/HoneyPaptan/hp-harness/internal/host"
	"github.com/HoneyPaptan/hp-harness/internal/journal"
	"github.com/HoneyPaptan/hp-harness/internal/provider"
	"github.com/HoneyPaptan/hp-harness/internal/provider/fake"
	"github.com/HoneyPaptan/hp-harness/internal/tool"
)

func continueCfg(limit int) Config {
	return Config{
		RunID: "r", Model: "m", System: "s", Dir: "/w",
		JournalPath: "/j", Prompt: "first", StepLimit: limit,
	}
}

func builtinDeps(h host.Host, fp *fake.Provider) Deps {
	return Deps{Host: h, Provider: fp, Tools: tool.NewRegistry(tool.Builtins()...)}
}

func mustFinishRun(t *testing.T, h host.Host, limit int, turns ...fake.Turn) {
	t.Helper()
	fp := &fake.Provider{Turns: turns}
	if _, err := Run(context.Background(), builtinDeps(h, fp), continueCfg(limit)); err != nil {
		t.Fatal(err)
	}
}

func readCall(id string) provider.ToolCall {
	return provider.ToolCall{ID: id, Name: "read", Args: json.RawMessage(`{"path":"nope"}`)}
}

func assertValidSequence(t *testing.T, msgs []provider.Message) {
	t.Helper()
	for i, m := range msgs {
		if len(m.ToolCalls) == 0 {
			continue
		}
		if i+1 >= len(msgs) {
			t.Fatalf("message %d: tool calls at end of history", i)
		}
		got := make(map[string]bool)
		for _, tr := range msgs[i+1].ToolResults {
			got[tr.CallID] = true
		}
		for _, c := range m.ToolCalls {
			if !got[c.ID] {
				t.Fatalf("message %d: call %s has no result in %+v", i, c.ID, msgs[i+1])
			}
		}
	}
}

func TestContinueSharesHistory(t *testing.T) {
	h := host.NewMemory()
	mustFinishRun(t, h, 5, fake.TextTurn("one"))
	fp := &fake.Provider{Turns: []fake.Turn{fake.TextTurn("two")}}
	out, err := Continue(context.Background(), builtinDeps(h, fp), "/j", "second")
	if err != nil {
		t.Fatal(err)
	}
	if out.Text != "two" || out.Reason != ReasonFinal {
		t.Fatalf("out %+v", out)
	}
	want := []provider.Message{
		{Role: provider.RoleUser, Text: "first"},
		{Role: provider.RoleAssistant, Text: "one"},
		{Role: provider.RoleUser, Text: "second"},
	}
	if got := fp.Requests[0].Messages; !reflect.DeepEqual(got, want) {
		t.Fatalf("messages %+v", got)
	}
	st := replayJournal(t, h)
	if !st.Finished || len(st.Messages) != 4 {
		t.Fatalf("state %+v", st)
	}
}

func replayJournal(t *testing.T, h host.Host) journal.State {
	t.Helper()
	recs, err := journal.Read(h, "/j")
	if err != nil {
		t.Fatal(err)
	}
	st, err := journal.Replay(recs)
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func TestContinueCrashThenResume(t *testing.T) {
	boom := errors.New("boom")
	tests := []struct {
		name  string
		turns []fake.Turn
	}{
		{"before any reply", []fake.Turn{{Err: boom}}},
		{"after a tool step", []fake.Turn{fake.ToolTurn(readCall("c")), {Err: boom}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := host.NewMemory()
			mustFinishRun(t, h, 5, fake.TextTurn("one"))
			fp := &fake.Provider{Turns: tt.turns}
			if _, err := Continue(context.Background(), builtinDeps(h, fp), "/j", "second"); !errors.Is(err, boom) {
				t.Fatalf("err %v", err)
			}
			if st := replayJournal(t, h); st.Finished {
				t.Fatal("run should be open")
			}
			again := &fake.Provider{Turns: []fake.Turn{fake.TextTurn("two")}}
			out, err := Resume(context.Background(), builtinDeps(h, again), "/j")
			if err != nil {
				t.Fatal(err)
			}
			if out.Text != "two" || out.Reason != ReasonFinal {
				t.Fatalf("out %+v", out)
			}
			sent := again.Requests[0].Messages
			if sent[2].Text != "second" {
				t.Fatalf("messages %+v", sent)
			}
			assertValidSequence(t, sent)
		})
	}
}

func TestResumeAfterContinueFinishedReturnsErrFinished(t *testing.T) {
	h := host.NewMemory()
	mustFinishRun(t, h, 5, fake.TextTurn("one"))
	fp := &fake.Provider{Turns: []fake.Turn{fake.TextTurn("two")}}
	if _, err := Continue(context.Background(), builtinDeps(h, fp), "/j", "second"); err != nil {
		t.Fatal(err)
	}
	if _, err := Resume(context.Background(), builtinDeps(h, fp), "/j"); !errors.Is(err, ErrFinished) {
		t.Fatalf("err %v", err)
	}
}

func TestContinueUnfinishedRefused(t *testing.T) {
	h := host.NewMemory()
	fp := &fake.Provider{Turns: []fake.Turn{{Err: errors.New("boom")}}}
	if _, err := Run(context.Background(), builtinDeps(h, fp), continueCfg(5)); err == nil {
		t.Fatal("expected error")
	}
	before, err := h.ReadFile("/j")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Continue(context.Background(), builtinDeps(h, fp), "/j", "second"); !errors.Is(err, ErrUnfinished) {
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

func interruptingTools(cancel context.CancelFunc, ran *atomic.Int32) *tool.Registry {
	stop := tool.Tool{
		Name: "stop", Schema: json.RawMessage(`{"type":"object"}`),
		Run: func(ctx context.Context, env tool.Env, raw json.RawMessage) (tool.Result, error) {
			cancel()
			return tool.Result{}, context.Canceled
		},
	}
	never := tool.Tool{
		Name: "never", Schema: json.RawMessage(`{"type":"object"}`),
		Run: func(ctx context.Context, env tool.Env, raw json.RawMessage) (tool.Result, error) {
			ran.Add(1)
			return tool.Result{Text: "ran"}, nil
		},
	}
	return tool.NewRegistry(stop, never)
}

func TestContinueInterruptedClosesDanglingCalls(t *testing.T) {
	h := host.NewMemory()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var ran atomic.Int32
	reg := interruptingTools(cancel, &ran)
	calls := []provider.ToolCall{
		{ID: "a", Name: "stop", Args: json.RawMessage(`{}`)},
		{ID: "b", Name: "never", Args: json.RawMessage(`{}`)},
	}
	fp := &fake.Provider{Turns: []fake.Turn{fake.ToolTurn(calls...)}}
	if _, err := Run(ctx, Deps{Host: h, Provider: fp, Tools: reg}, continueCfg(5)); !errors.Is(err, context.Canceled) {
		t.Fatalf("err %v", err)
	}
	next := &fake.Provider{Turns: []fake.Turn{fake.TextTurn("ok")}}
	d := Deps{Host: h, Provider: next, Tools: reg}
	if _, err := Continue(context.Background(), d, "/j", "second"); !errors.Is(err, ErrUnfinished) {
		t.Fatalf("err %v", err)
	}
	out, err := ContinueInterrupted(context.Background(), d, "/j", "second")
	if err != nil {
		t.Fatal(err)
	}
	if out.Text != "ok" {
		t.Fatalf("out %+v", out)
	}
	if ran.Load() != 0 {
		t.Fatal("unstarted tool ran after interrupt")
	}
	sent := next.Requests[0].Messages
	assertValidSequence(t, sent)
	results := sent[2].ToolResults
	want := []provider.ToolResult{
		{CallID: "a", Text: stoppedToolMsg, IsError: true},
		{CallID: "b", Text: skippedToolMsg, IsError: true},
	}
	if !reflect.DeepEqual(results, want) {
		t.Fatalf("results %+v", results)
	}
	if sent[3].Text != "second" {
		t.Fatalf("messages %+v", sent)
	}
	if st := replayJournal(t, h); !reflect.DeepEqual(st.Messages[:4], sent) {
		t.Fatalf("replay %+v", st.Messages)
	}
}

func TestContinueInterruptedStates(t *testing.T) {
	assistant := provider.Message{Role: provider.RoleAssistant, Text: "answer"}
	tests := []struct {
		name   string
		write  func(*journal.Writer) error
		wantN  int
		before string
	}{
		{"no reply yet", func(w *journal.Writer) error { return nil }, 2, "first"},
		{"final reply without run_end", func(w *journal.Writer) error { return w.Assistant(assistant) }, 3, "answer"},
		{"finished", func(w *journal.Writer) error {
			if err := w.Assistant(assistant); err != nil {
				return err
			}
			return w.End(ReasonFinal)
		}, 3, "answer"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := host.NewMemory()
			w, err := seed(Deps{Host: h}, continueCfg(5))
			if err != nil {
				t.Fatal(err)
			}
			if err := tt.write(w); err != nil {
				t.Fatal(err)
			}
			fp := &fake.Provider{Turns: []fake.Turn{fake.TextTurn("ok")}}
			if _, err := ContinueInterrupted(context.Background(), builtinDeps(h, fp), "/j", "second"); err != nil {
				t.Fatal(err)
			}
			sent := fp.Requests[0].Messages
			if len(sent) != tt.wantN || sent[len(sent)-2].Text != tt.before || sent[len(sent)-1].Text != "second" {
				t.Fatalf("messages %+v", sent)
			}
		})
	}
}

func TestContinueClosesCallsReusingAnEarlierID(t *testing.T) {
	h := host.NewMemory()
	mustFinishRun(t, h, 5, fake.ToolTurn(readCall("same")), fake.TextTurn("one"))
	w, _, err := journal.Open(h, "/j")
	if err != nil {
		t.Fatal(err)
	}
	if err := w.User("second"); err != nil {
		t.Fatal(err)
	}
	reused := provider.Message{Role: provider.RoleAssistant, ToolCalls: []provider.ToolCall{readCall("same")}}
	if err := w.Assistant(reused); err != nil {
		t.Fatal(err)
	}
	fp := &fake.Provider{Turns: []fake.Turn{fake.TextTurn("ok")}}
	if _, err := ContinueInterrupted(context.Background(), builtinDeps(h, fp), "/j", "third"); err != nil {
		t.Fatal(err)
	}
	assertValidSequence(t, fp.Requests[0].Messages)
}

func TestContinueOldJournal(t *testing.T) {
	fixture := `{"seq":0,"ts":"2026-03-15T10:00:00Z","type":"run_start","data":{"run_id":"old","model":"a","system":"s","dir":"/w","step_limit":5,"preset":"p","base_url":"","key_env":"","max_tokens":77,"fallbacks":["p/b"],"jev":true}}
{"seq":1,"ts":"2026-03-15T10:00:01Z","type":"user","data":{"text":"first"}}
{"seq":2,"ts":"2026-03-15T10:00:02Z","type":"model_switch","data":{"from":"p/a","to":"p/b"}}
{"seq":3,"ts":"2026-03-15T10:00:03Z","type":"assistant","data":{"message":{"Role":"assistant","Text":"one"}}}
{"seq":4,"ts":"2026-03-15T10:00:04Z","type":"run_end","data":{"reason":"final"}}
`
	h := host.NewMemory()
	if err := h.WriteFile("/j", []byte(fixture)); err != nil {
		t.Fatal(err)
	}
	st := replayJournal(t, h)
	if !st.Finished || st.EndReason != ReasonFinal || st.Steps != 1 || !st.Start.Jev {
		t.Fatalf("state %+v", st)
	}
	fp := &fake.Provider{Turns: []fake.Turn{fake.TextTurn("two")}}
	var specs []string
	d := Deps{
		Host:  h,
		Tools: tool.NewRegistry(tool.Builtins()...),
		Providers: func(spec string) (provider.Provider, string, error) {
			specs = append(specs, spec)
			return fp, "wire-" + spec, nil
		},
	}
	if _, err := Continue(context.Background(), d, "/j", "second"); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(specs, []string{"p/b"}) {
		t.Fatalf("specs %v", specs)
	}
	req := fp.Requests[0]
	if req.Model != "wire-p/b" || req.MaxTokens != 77 || len(req.Messages) != 3 {
		t.Fatalf("request %+v", req)
	}
	recs, err := journal.Read(h, "/j")
	if err != nil {
		t.Fatal(err)
	}
	for i, rec := range recs {
		if rec.Seq != i {
			t.Fatalf("record %d seq %d", i, rec.Seq)
		}
	}
}

func TestContinueStepLimitPerPrompt(t *testing.T) {
	tools := func(n int) []fake.Turn {
		turns := make([]fake.Turn, n)
		for i := range turns {
			turns[i] = fake.ToolTurn(readCall("x"))
		}
		return turns
	}
	tests := []struct {
		name       string
		first      []fake.Turn
		second     []fake.Turn
		wantReason string
		wantSteps  int
	}{
		{"fresh budget after a step limit", tools(2), tools(2), ReasonStepLimit, 2},
		{"steps from the first prompt do not count", append(tools(1), fake.TextTurn("one")), append(tools(1), fake.TextTurn("two")), ReasonFinal, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := host.NewMemory()
			mustFinishRun(t, h, 2, tt.first...)
			fp := &fake.Provider{Turns: tt.second}
			out, err := Continue(context.Background(), builtinDeps(h, fp), "/j", "second")
			if err != nil {
				t.Fatal(err)
			}
			if out.Reason != tt.wantReason || out.Steps != tt.wantSteps {
				t.Fatalf("out %+v", out)
			}
			assertValidSequence(t, fp.Requests[len(fp.Requests)-1].Messages)
		})
	}
}

func TestResumeStepLimitAfterContinue(t *testing.T) {
	h := host.NewMemory()
	mustFinishRun(t, h, 2, fake.ToolTurn(readCall("x")), fake.TextTurn("one"))
	fp := &fake.Provider{Turns: []fake.Turn{fake.ToolTurn(readCall("y")), {Err: errors.New("boom")}}}
	if _, err := Continue(context.Background(), builtinDeps(h, fp), "/j", "second"); err == nil {
		t.Fatal("expected error")
	}
	if st := replayJournal(t, h); st.Steps != 1 {
		t.Fatalf("steps %d", st.Steps)
	}
	again := &fake.Provider{Turns: []fake.Turn{fake.ToolTurn(readCall("z"))}}
	out, err := Resume(context.Background(), builtinDeps(h, again), "/j")
	if err != nil {
		t.Fatal(err)
	}
	if out.Reason != ReasonStepLimit || out.Steps != 2 {
		t.Fatalf("out %+v", out)
	}
}
