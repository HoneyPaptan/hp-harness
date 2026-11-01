package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/HoneyPaptan/hp-harness/internal/host"
	"github.com/HoneyPaptan/hp-harness/internal/provider"
	"github.com/HoneyPaptan/hp-harness/internal/provider/fake"
	"github.com/HoneyPaptan/hp-harness/internal/tool"
)

func connectedTools(n int) []tool.Tool {
	out := make([]tool.Tool, n)
	for i := range out {
		name := fmt.Sprintf("srv__t%d", i)
		out[i] = tool.Tool{
			Name: name, Description: "remote " + name, Schema: json.RawMessage(`{"type":"object","properties":{}}`),
			Run: func(context.Context, tool.Env, json.RawMessage) (tool.Result, error) {
				return tool.Result{Text: "ran " + name}, nil
			},
		}
	}
	return out
}

func deferredRegistry(n int) *tool.Registry {
	return tool.NewRegistry(tool.Compose(tool.Builtins(), connectedTools(n))...)
}

func searchCall(id, query string) provider.ToolCall {
	return provider.ToolCall{ID: id, Name: tool.SearchName, Args: json.RawMessage(`{"query":"` + query + `"}`)}
}

func remoteCall(id, name string) provider.ToolCall {
	return provider.ToolCall{ID: id, Name: name, Args: json.RawMessage(`{}`)}
}

func deferredDeps(h *host.Memory, fp *fake.Provider, reg *tool.Registry) Deps {
	return Deps{Host: h, Provider: fp, Tools: reg}
}

func deferredCfg() Config {
	return Config{RunID: "r", Model: "m", System: "s", Dir: "/w", JournalPath: "/j", Prompt: "go", StepLimit: 10}
}

func lastResult(t *testing.T, req provider.Request) provider.ToolResult {
	t.Helper()
	m := req.Messages[len(req.Messages)-1]
	if len(m.ToolResults) == 0 {
		t.Fatalf("no result in %+v", m)
	}
	return m.ToolResults[len(m.ToolResults)-1]
}

func TestFoundThenCalled(t *testing.T) {
	fp := &fake.Provider{Turns: []fake.Turn{
		fake.ToolTurn(searchCall("1", "srv__t3")),
		fake.ToolTurn(remoteCall("2", "srv__t3")),
		fake.TextTurn("done"),
	}}
	out, err := Run(context.Background(), deferredDeps(host.NewMemory(), fp, deferredRegistry(6)), deferredCfg())
	if err != nil || out.Reason != ReasonFinal {
		t.Fatalf("out %+v err %v", out, err)
	}
	reqs := fp.Requests
	base := []string{"read", "write", "edit", "bash", "grep", "tools"}
	if got := requestNames(reqs[0]); !slices.Equal(got, base) {
		t.Fatalf("first request %v", got)
	}
	want := append(slices.Clone(base), "srv__t3")
	for _, r := range reqs[1:] {
		if got := requestNames(r); !slices.Equal(got, want) {
			t.Fatalf("request %v", got)
		}
	}
	if got := lastResult(t, reqs[2]); got.Text != "ran srv__t3" {
		t.Fatalf("result %+v", got)
	}
}

func TestLoadedToolsKeepThePrefixStable(t *testing.T) {
	fp := &fake.Provider{Turns: []fake.Turn{
		fake.ToolTurn(searchCall("1", "srv__t5")),
		fake.ToolTurn(searchCall("2", "srv__t0")),
		fake.ToolTurn(searchCall("3", "srv__t5")),
		fake.TextTurn("done"),
	}}
	if _, err := Run(context.Background(), deferredDeps(host.NewMemory(), fp, deferredRegistry(6)), deferredCfg()); err != nil {
		t.Fatal(err)
	}
	got := requestNames(fp.Requests[3])
	want := []string{"read", "write", "edit", "bash", "grep", "tools", "srv__t5", "srv__t0"}
	if !slices.Equal(got, want) {
		t.Fatalf("tools %v", got)
	}
}

func TestCallingAnUnloadedToolStillRuns(t *testing.T) {
	fp := &fake.Provider{Turns: []fake.Turn{
		fake.ToolTurn(remoteCall("1", "srv__t2")),
		fake.TextTurn("done"),
	}}
	if _, err := Run(context.Background(), deferredDeps(host.NewMemory(), fp, deferredRegistry(6)), deferredCfg()); err != nil {
		t.Fatal(err)
	}
	if got := lastResult(t, fp.Requests[1]); got.IsError || got.Text != "ran srv__t2" {
		t.Fatalf("result %+v", got)
	}
}

func TestLoadedToolsSurviveResume(t *testing.T) {
	h := host.NewMemory()
	reg := deferredRegistry(6)
	first := &fake.Provider{Turns: []fake.Turn{
		fake.ToolTurn(searchCall("1", "srv__t4")),
		{Err: errors.New("process died")},
	}}
	if _, err := Run(context.Background(), deferredDeps(h, first, reg), deferredCfg()); err == nil {
		t.Fatal("want first run to stop")
	}
	second := &fake.Provider{Turns: []fake.Turn{
		fake.ToolTurn(remoteCall("2", "srv__t4")),
		fake.TextTurn("done"),
	}}
	out, err := Resume(context.Background(), deferredDeps(h, second, reg), "/j")
	if err != nil || out.Reason != ReasonFinal {
		t.Fatalf("out %+v err %v", out, err)
	}
	for _, r := range second.Requests {
		if !slices.Contains(requestNames(r), "srv__t4") {
			t.Fatalf("lost after resume: %v", requestNames(r))
		}
	}
}

func TestRemovedConnectionDoesNotBreakResume(t *testing.T) {
	h := host.NewMemory()
	first := &fake.Provider{Turns: []fake.Turn{
		fake.ToolTurn(searchCall("1", "srv__t4")),
		fake.ToolTurn(remoteCall("2", "srv__t4")),
		{Err: errors.New("process died")},
	}}
	if _, err := Run(context.Background(), deferredDeps(h, first, deferredRegistry(6)), deferredCfg()); err == nil {
		t.Fatal("want first run to stop")
	}
	second := &fake.Provider{Turns: []fake.Turn{
		fake.ToolTurn(remoteCall("3", "srv__t4")),
		fake.TextTurn("done"),
	}}
	out, err := Resume(context.Background(), deferredDeps(h, second, deferredRegistry(0)), "/j")
	if err != nil || out.Reason != ReasonFinal {
		t.Fatalf("out %+v err %v", out, err)
	}
	if names := requestNames(second.Requests[0]); slices.Contains(names, "srv__t4") {
		t.Fatalf("removed tool still sent: %v", names)
	}
	flat := fmt.Sprintf("%+v", second.Requests[0].Messages)
	if !strings.Contains(flat, "ran srv__t4") {
		t.Fatalf("history lost: %s", flat)
	}
	got := lastResult(t, second.Requests[1])
	if !got.IsError || !strings.Contains(got.Text, "unknown tool") {
		t.Fatalf("result %+v", got)
	}
}

func TestPendingCallToRemovedConnectionGetsAnError(t *testing.T) {
	h := host.NewMemory()
	blocked := connectedTools(6)
	blocked[1].Run = func(context.Context, tool.Env, json.RawMessage) (tool.Result, error) {
		return tool.Result{}, errors.New("host failed")
	}
	reg := tool.NewRegistry(tool.Compose(tool.Builtins(), blocked)...)
	first := &fake.Provider{Turns: []fake.Turn{
		fake.ToolTurn(remoteCall("1", "srv__t1"), remoteCall("2", "srv__t2")),
	}}
	if _, err := Run(context.Background(), deferredDeps(h, first, reg), deferredCfg()); err == nil {
		t.Fatal("want first run to stop")
	}
	second := &fake.Provider{Turns: []fake.Turn{fake.TextTurn("done")}}
	out, err := Resume(context.Background(), deferredDeps(h, second, deferredRegistry(0)), "/j")
	if err != nil || out.Reason != ReasonFinal {
		t.Fatalf("out %+v err %v", out, err)
	}
	got := lastResult(t, second.Requests[0])
	if !got.IsError || !strings.Contains(got.Text, "unknown tool") {
		t.Fatalf("result %+v", got)
	}
}

func TestChooserSeesEveryToolWithoutDeferral(t *testing.T) {
	all := slices.Concat(tool.Builtins(), connectedTools(6))
	reg := tool.NewRegistry(all...)
	var seen int
	chooser := &fakeChooser{keep: map[string]bool{"read": true}}
	fp := &fake.Provider{Turns: []fake.Turn{fake.TextTurn("done")}}
	chooser.hook = func() { seen = len(reg.Defs()) }
	d := Deps{Host: host.NewMemory(), Provider: fp, Tools: reg, ToolChooser: chooser}
	if _, err := Run(context.Background(), d, deferredCfg()); err != nil {
		t.Fatal(err)
	}
	if seen != 11 || !slices.Equal(requestNames(fp.Requests[0]), []string{"read"}) {
		t.Fatalf("seen %d request %v", seen, requestNames(fp.Requests[0]))
	}
}

func TestLoadedToolsIgnoresOtherResults(t *testing.T) {
	loaded := "loaded: srv__t1\n"
	reused := []provider.Message{
		{Role: provider.RoleAssistant, ToolCalls: []provider.ToolCall{searchCall("1", "a")}},
		{Role: provider.RoleUser, ToolResults: []provider.ToolResult{{CallID: "1", Text: "no tools match"}}},
		{Role: provider.RoleAssistant, ToolCalls: []provider.ToolCall{remoteCall("1", "srv__t0")}},
		{Role: provider.RoleUser, ToolResults: []provider.ToolResult{{CallID: "1", Text: loaded}}},
	}
	tests := []struct {
		name string
		msgs []provider.Message
		want []string
	}{
		{name: "another tool prints the same text", msgs: []provider.Message{
			{Role: provider.RoleAssistant, ToolCalls: []provider.ToolCall{remoteCall("1", "bash")}},
			{Role: provider.RoleUser, ToolResults: []provider.ToolResult{{CallID: "1", Text: loaded}}},
		}},
		{name: "reused call id", msgs: reused},
		{name: "failed search", msgs: []provider.Message{
			{Role: provider.RoleAssistant, ToolCalls: []provider.ToolCall{searchCall("1", "a")}},
			{Role: provider.RoleUser, ToolResults: []provider.ToolResult{{CallID: "1", Text: loaded, IsError: true}}},
		}},
		{name: "real search", want: []string{"srv__t1"}, msgs: []provider.Message{
			{Role: provider.RoleAssistant, ToolCalls: []provider.ToolCall{searchCall("1", "a")}},
			{Role: provider.RoleUser, ToolResults: []provider.ToolResult{{CallID: "1", Text: loaded}}},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := loadedTools(tt.msgs); !slices.Equal(got, tt.want) {
				t.Fatalf("got %v", got)
			}
		})
	}
}
