package core

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/HoneyPaptan/hp-harness/internal/host"
	"github.com/HoneyPaptan/hp-harness/internal/journal"
	"github.com/HoneyPaptan/hp-harness/internal/provider"
	"github.com/HoneyPaptan/hp-harness/internal/provider/fake"
	"github.com/HoneyPaptan/hp-harness/internal/tool"
)

func fallbackDeps(primary, fb *fake.Provider) Deps {
	return Deps{
		Providers: func(spec string) (provider.Provider, string, error) {
			switch spec {
			case "groq/primary":
				return primary, "primary-id", nil
			case "groq/fallback":
				return fb, "fallback-id", nil
			default:
				return nil, "", errors.New("unknown spec")
			}
		},
	}
}

func fallbackCfg() Config {
	return Config{
		RunID: "r", Model: "primary", System: "s", Dir: "/w", Preset: "groq",
		JournalPath: "/j", Prompt: "go", StepLimit: 5,
		Fallbacks: []string{"groq/fallback"}, SwitchAfter: 20 * time.Second,
	}
}

func TestSwitchOnLongLimit(t *testing.T) {
	h := host.NewMemory()
	reg := tool.NewRegistry(tool.Builtins()...)
	primary := &fake.Provider{Turns: []fake.Turn{
		{Err: &provider.RateLimitError{RetryAfter: 5 * time.Minute}},
	}}
	fb := &fake.Provider{Turns: []fake.Turn{fake.TextTurn("ok")}}
	d := fallbackDeps(primary, fb)
	d.Host = h
	d.Tools = reg
	if _, err := Run(context.Background(), d, fallbackCfg()); err != nil {
		t.Fatal(err)
	}
	if h.Slept != 0 {
		t.Fatalf("slept %v", h.Slept)
	}
	if len(fb.Requests) != 1 || fb.Requests[0].Model != "fallback-id" {
		t.Fatalf("fb reqs %+v", fb.Requests)
	}
	recs, err := journal.Read(h, "/j")
	if err != nil {
		t.Fatal(err)
	}
	var switches int
	for _, rec := range recs {
		if rec.Type == journal.TypeModelSwitch {
			switches++
		}
	}
	if switches != 1 {
		t.Fatalf("switches %d", switches)
	}
}

func TestShortLimitWaitsSameModel(t *testing.T) {
	h := host.NewMemory()
	reg := tool.NewRegistry(tool.Builtins()...)
	primary := &fake.Provider{Turns: []fake.Turn{
		{Err: &provider.RateLimitError{RetryAfter: 10 * time.Second}},
		fake.TextTurn("ok"),
	}}
	fb := &fake.Provider{}
	d := fallbackDeps(primary, fb)
	d.Host = h
	d.Tools = reg
	if _, err := Run(context.Background(), d, fallbackCfg()); err != nil {
		t.Fatal(err)
	}
	if h.Slept != 10*time.Second {
		t.Fatalf("slept %v", h.Slept)
	}
	if len(primary.Requests) != 2 || len(fb.Requests) != 0 {
		t.Fatalf("primary %d fb %d", len(primary.Requests), len(fb.Requests))
	}
}

func TestAllLimitedWaitsEarliest(t *testing.T) {
	h := host.NewMemory()
	reg := tool.NewRegistry(tool.Builtins()...)
	primary := &fake.Provider{Turns: []fake.Turn{
		{Err: &provider.RateLimitError{RetryAfter: 5 * time.Minute}},
	}}
	fb := &fake.Provider{Turns: []fake.Turn{
		{Err: &provider.RateLimitError{RetryAfter: 2 * time.Minute}},
		fake.TextTurn("ok"),
	}}
	d := fallbackDeps(primary, fb)
	d.Host = h
	d.Tools = reg
	if _, err := Run(context.Background(), d, fallbackCfg()); err != nil {
		t.Fatal(err)
	}
	if h.Slept != 2*time.Minute {
		t.Fatalf("slept %v", h.Slept)
	}
	if len(fb.Requests) != 2 || fb.Requests[1].Model != "fallback-id" {
		t.Fatalf("fb reqs %d", len(fb.Requests))
	}
}

func TestResumeAfterSwitch(t *testing.T) {
	h := host.NewMemory()
	reg := tool.NewRegistry(tool.Builtins()...)
	start := journal.RunStart{
		RunID: "r", Model: "primary", System: "s", Dir: "/w", StepLimit: 5, Preset: "groq",
		Fallbacks: []string{"groq/fallback"},
	}
	w, err := journal.Create(h, "/j", start)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.User("go"); err != nil {
		t.Fatal(err)
	}
	if err := w.ModelSwitch("groq/primary", "groq/fallback"); err != nil {
		t.Fatal(err)
	}
	fb := &fake.Provider{Turns: []fake.Turn{fake.TextTurn("done")}}
	d := fallbackDeps(&fake.Provider{}, fb)
	d.Host = h
	d.Tools = reg
	out, err := Resume(context.Background(), d, "/j")
	if err != nil {
		t.Fatal(err)
	}
	if out.Text != "done" {
		t.Fatal(out)
	}
	if len(fb.Requests) != 1 || fb.Requests[0].Model != "fallback-id" {
		t.Fatalf("req model %q", fb.Requests[0].Model)
	}
}

func TestProvidersErrorBeforeJournal(t *testing.T) {
	h := host.NewMemory()
	reg := tool.NewRegistry(tool.Builtins()...)
	want := errors.New("no key")
	d := Deps{
		Host:  h,
		Tools: reg,
		Providers: func(spec string) (provider.Provider, string, error) {
			return nil, "", want
		},
	}
	cfg := fallbackCfg()
	_, err := Run(context.Background(), d, cfg)
	if err != want {
		t.Fatalf("err %v", err)
	}
	if _, err := h.Stat("/j"); err == nil {
		t.Fatal("journal created")
	}
}

func TestLongLimitSwitchesDespiteMaxWait(t *testing.T) {
	h := host.NewMemory()
	reg := tool.NewRegistry(tool.Builtins()...)
	primary := &fake.Provider{Turns: []fake.Turn{
		{Err: &provider.RateLimitError{RetryAfter: 7 * time.Hour}},
	}}
	fb := &fake.Provider{Turns: []fake.Turn{fake.TextTurn("ok")}}
	d := fallbackDeps(primary, fb)
	d.Host = h
	d.Tools = reg
	cfg := fallbackCfg()
	cfg.MaxWait = 6 * time.Hour
	out, err := Run(context.Background(), d, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if out.Reason != ReasonFinal || h.Slept != 0 {
		t.Fatalf("out %+v slept %v", out, h.Slept)
	}
	if len(fb.Requests) != 1 {
		t.Fatalf("fb reqs %d", len(fb.Requests))
	}
}

func TestProvidersNilNoSwitch(t *testing.T) {
	h := host.NewMemory()
	reg := tool.NewRegistry(tool.Builtins()...)
	fp := &fake.Provider{Turns: []fake.Turn{
		{Err: &provider.RateLimitError{RetryAfter: 5 * time.Minute}},
		fake.TextTurn("ok"),
	}}
	cfg := fallbackCfg()
	if _, err := Run(context.Background(), Deps{Host: h, Provider: fp, Tools: reg}, cfg); err != nil {
		t.Fatal(err)
	}
	if h.Slept != 5*time.Minute {
		t.Fatalf("slept %v", h.Slept)
	}
	recs, _ := journal.Read(h, "/j")
	for _, rec := range recs {
		if rec.Type == journal.TypeModelSwitch {
			t.Fatal("unexpected switch")
		}
	}
}
