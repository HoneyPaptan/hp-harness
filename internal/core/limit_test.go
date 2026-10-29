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

func limitCfg(h *host.Memory) Config {
	return Config{
		RunID: "r", Model: "gpt", System: "s", Dir: "/w", Preset: "groq",
		JournalPath: "/j", Prompt: "go", StepLimit: 5,
	}
}

func TestWaitThenFinal(t *testing.T) {
	h := host.NewMemory()
	reg := tool.NewRegistry(tool.Builtins()...)
	fp := &fake.Provider{Turns: []fake.Turn{
		{Err: &provider.RateLimitError{RetryAfter: 30 * time.Second}},
		fake.TextTurn("done"),
	}}
	var waited time.Duration
	out, err := Run(context.Background(), Deps{
		Host: h, Provider: fp, Tools: reg,
		Observer: Observer{OnWait: func(spec string, d time.Duration, until time.Time) {
			waited = d
		}},
	}, limitCfg(h))
	if err != nil {
		t.Fatal(err)
	}
	if out.Reason != ReasonFinal || h.Slept != 30*time.Second || waited != 30*time.Second {
		t.Fatalf("out %+v slept %v waited %v", out, h.Slept, waited)
	}
	recs, err := journal.Read(h, "/j")
	if err != nil {
		t.Fatal(err)
	}
	var waits int
	for _, rec := range recs {
		if rec.Type == journal.TypeLimitWait {
			waits++
		}
	}
	if waits != 1 {
		t.Fatalf("limit_wait count %d", waits)
	}
}

func TestUnknownDelayBackoff(t *testing.T) {
	h := host.NewMemory()
	reg := tool.NewRegistry(tool.Builtins()...)
	fp := &fake.Provider{Turns: []fake.Turn{
		{Err: &provider.RateLimitError{}},
		{Err: &provider.RateLimitError{}},
		fake.TextTurn("ok"),
	}}
	if _, err := Run(context.Background(), Deps{Host: h, Provider: fp, Tools: reg}, limitCfg(h)); err != nil {
		t.Fatal(err)
	}
	if h.Slept != 6*time.Second {
		t.Fatalf("slept %v", h.Slept)
	}
}

func TestLimitTooLong(t *testing.T) {
	h := host.NewMemory()
	reg := tool.NewRegistry(tool.Builtins()...)
	fp := &fake.Provider{Turns: []fake.Turn{
		{Err: &provider.RateLimitError{RetryAfter: 7 * time.Hour}},
	}}
	cfg := limitCfg(h)
	cfg.MaxWait = 6 * time.Hour
	_, err := Run(context.Background(), Deps{Host: h, Provider: fp, Tools: reg}, cfg)
	if !errors.Is(err, ErrLimitTooLong) {
		t.Fatalf("err %v", err)
	}
	recs, err := journal.Read(h, "/j")
	if err != nil {
		t.Fatal(err)
	}
	for _, rec := range recs {
		if rec.Type == journal.TypeRunEnd {
			t.Fatal("run_end written")
		}
	}
}

func TestCancelDuringWait(t *testing.T) {
	h := host.NewMemory()
	reg := tool.NewRegistry(tool.Builtins()...)
	fp := &fake.Provider{Turns: []fake.Turn{
		{Err: &provider.RateLimitError{RetryAfter: time.Minute}},
	}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Run(ctx, Deps{Host: h, Provider: fp, Tools: reg}, limitCfg(h))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err %v", err)
	}
}

func TestRunStartLimitFieldsRoundTrip(t *testing.T) {
	h := host.NewMemory()
	reg := tool.NewRegistry(tool.Builtins()...)
	fp := &fake.Provider{Turns: []fake.Turn{fake.TextTurn("x")}}
	cfg := limitCfg(h)
	cfg.Fallbacks = []string{"groq/other"}
	cfg.SwitchAfter = 15 * time.Second
	cfg.MaxWait = time.Hour
	if _, err := Run(context.Background(), Deps{Host: h, Provider: fp, Tools: reg}, cfg); err != nil {
		t.Fatal(err)
	}
	recs, err := journal.Read(h, "/j")
	if err != nil {
		t.Fatal(err)
	}
	st, err := journal.Replay(recs)
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Start.Fallbacks) != 1 || st.Start.SwitchAfter != 15*time.Second || st.Start.MaxWait != time.Hour {
		t.Fatalf("start %+v", st.Start)
	}
}
