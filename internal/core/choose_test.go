package core

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/HoneyPaptan/hp-harness/internal/host"
	"github.com/HoneyPaptan/hp-harness/internal/provider"
	"github.com/HoneyPaptan/hp-harness/internal/provider/fake"
	"github.com/HoneyPaptan/hp-harness/internal/tool"
)

type fakeChooser struct {
	keep  map[string]bool
	calls int
	seen  []int
	hook  func()
}

func (f *fakeChooser) Choose(ctx context.Context, msgs []provider.Message, defs []provider.ToolDef) []provider.ToolDef {
	f.calls++
	f.seen = append(f.seen, len(msgs))
	if f.hook != nil {
		f.hook()
	}
	var out []provider.ToolDef
	for _, d := range defs {
		if f.keep[d.Name] {
			out = append(out, d)
		}
	}
	return out
}

func chooseCfg() Config {
	return Config{
		RunID: "r", Model: "m", System: "s", Dir: "/w",
		JournalPath: "/j", Prompt: "go", StepLimit: 5,
	}
}

func chooseProvider() *fake.Provider {
	call := provider.ToolCall{ID: "1", Name: "grep", Args: json.RawMessage(`{"pattern":"x"}`)}
	return &fake.Provider{Turns: []fake.Turn{fake.ToolTurn(call), fake.TextTurn("done")}}
}

func requestNames(req provider.Request) []string {
	names := make([]string, len(req.Tools))
	for i, d := range req.Tools {
		names[i] = d.Name
	}
	return names
}

func TestToolChooserRequests(t *testing.T) {
	reg := tool.NewRegistry(tool.Builtins()...)
	all := requestNames(provider.Request{Tools: reg.Defs()})
	tests := []struct {
		name    string
		chooser ToolChooser
		want    []string
	}{
		{"nil sends all tools", nil, all},
		{"narrows the request", &fakeChooser{keep: map[string]bool{"read": true, "bash": true}}, []string{"read", "bash"}},
		{"empty choice sends all tools", &fakeChooser{}, all},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fp := chooseProvider()
			d := Deps{Host: host.NewMemory(), Provider: fp, Tools: reg, ToolChooser: tt.chooser}
			out, err := Run(context.Background(), d, chooseCfg())
			if err != nil {
				t.Fatal(err)
			}
			if out.Text != "done" || out.Reason != ReasonFinal || out.Steps != 1 {
				t.Fatalf("out %+v", out)
			}
			for _, req := range fp.Requests {
				if !reflect.DeepEqual(requestNames(req), tt.want) {
					t.Fatalf("tools %v, want %v", requestNames(req), tt.want)
				}
			}
		})
	}
}

func TestChooserAsksEachTurnWithHistory(t *testing.T) {
	reg := tool.NewRegistry(tool.Builtins()...)
	ch := &fakeChooser{keep: map[string]bool{"read": true}}
	d := Deps{Host: host.NewMemory(), Provider: chooseProvider(), Tools: reg, ToolChooser: ch}
	if _, err := Run(context.Background(), d, chooseCfg()); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(ch.seen, []int{1, 3}) {
		t.Fatalf("messages seen %v", ch.seen)
	}
}

func TestChooserAsksOnceAcrossLimitRetries(t *testing.T) {
	reg := tool.NewRegistry(tool.Builtins()...)
	ch := &fakeChooser{keep: map[string]bool{"read": true}}
	fp := &fake.Provider{Turns: []fake.Turn{
		{Err: &provider.RateLimitError{}},
		fake.TextTurn("ok"),
	}}
	h := host.NewMemory()
	d := Deps{Host: h, Provider: fp, Tools: reg, ToolChooser: ch}
	if _, err := Run(context.Background(), d, limitCfg(h)); err != nil {
		t.Fatal(err)
	}
	if ch.calls != 1 || len(fp.Requests) != 2 {
		t.Fatalf("calls %d requests %d", ch.calls, len(fp.Requests))
	}
	for _, req := range fp.Requests {
		if !reflect.DeepEqual(requestNames(req), []string{"read"}) {
			t.Fatalf("tools %v", requestNames(req))
		}
	}
}

func TestCancelDuringChoiceStopsRun(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reg := tool.NewRegistry(tool.Builtins()...)
	ch := &fakeChooser{keep: map[string]bool{"read": true}, hook: cancel}
	d := Deps{Host: host.NewMemory(), Provider: chooseProvider(), Tools: reg, ToolChooser: ch}
	if _, err := Run(ctx, d, chooseCfg()); !errors.Is(err, context.Canceled) {
		t.Fatalf("err %v", err)
	}
}
