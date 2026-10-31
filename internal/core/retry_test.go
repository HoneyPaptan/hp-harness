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

var errCut = errors.New("cut")

func partialTurn() fake.Turn {
	return fake.Turn{
		Events: []provider.Event{{Kind: provider.EventText, Text: "partial"}},
		Err:    &provider.TransientError{Err: errCut},
	}
}

func assistantTexts(t *testing.T, h *host.Memory) []string {
	t.Helper()
	recs, err := journal.Read(h, "/j")
	if err != nil {
		t.Fatal(err)
	}
	st, err := journal.Replay(recs)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, m := range st.Messages {
		if m.Role == provider.RoleAssistant {
			out = append(out, m.Text)
		}
	}
	return out
}

func TestTransientThenSuccess(t *testing.T) {
	h := host.NewMemory()
	fp := &fake.Provider{Turns: []fake.Turn{partialTurn(), fake.TextTurn("whole")}}
	var delays []time.Duration
	var attempts []int
	obs := Observer{OnRetry: func(d time.Duration, attempt, max int) {
		delays = append(delays, d)
		attempts = append(attempts, attempt)
		if max != maxStreamAttempts {
			t.Fatalf("max %d", max)
		}
	}}
	deps := Deps{Host: h, Provider: fp, Tools: tool.NewRegistry(tool.Builtins()...), Observer: obs}
	out, err := Run(context.Background(), deps, limitCfg(h))
	if err != nil {
		t.Fatal(err)
	}
	if out.Text != "whole" || out.Reason != ReasonFinal || len(fp.Requests) != 2 {
		t.Fatalf("out %+v requests %d", out, len(fp.Requests))
	}
	got := assistantTexts(t, h)
	if len(got) != 1 || got[0] != "whole" {
		t.Fatalf("assistant %q", got)
	}
	if len(delays) != 1 || delays[0] != 2*time.Second || attempts[0] != 1 || h.Slept != 2*time.Second {
		t.Fatalf("delays %v attempts %v slept %v", delays, attempts, h.Slept)
	}
}

func TestTransientRetriesRunOut(t *testing.T) {
	h := host.NewMemory()
	turns := make([]fake.Turn, 0, maxStreamAttempts+1)
	for i := 0; i < maxStreamAttempts; i++ {
		turns = append(turns, partialTurn())
	}
	fp := &fake.Provider{Turns: append(turns, fake.TextTurn("never"))}
	retries := 0
	obs := Observer{OnRetry: func(time.Duration, int, int) { retries++ }}
	deps := Deps{Host: h, Provider: fp, Tools: tool.NewRegistry(tool.Builtins()...), Observer: obs}
	_, err := Run(context.Background(), deps, limitCfg(h))
	if !errors.Is(err, errCut) {
		t.Fatalf("err %v", err)
	}
	if len(fp.Requests) != maxStreamAttempts || retries != maxStreamAttempts-1 {
		t.Fatalf("requests %d retries %d", len(fp.Requests), retries)
	}
	if h.Slept != 30*time.Second {
		t.Fatalf("slept %v", h.Slept)
	}
	recs, err := journal.Read(h, "/j")
	if err != nil {
		t.Fatal(err)
	}
	for _, rec := range recs {
		if rec.Type == journal.TypeAssistant || rec.Type == journal.TypeRunEnd {
			t.Fatalf("record %s", rec.Type)
		}
	}
}

func TestCancelDuringRetrySleep(t *testing.T) {
	h := host.NewMemory()
	fp := &fake.Provider{Turns: []fake.Turn{partialTurn(), fake.TextTurn("never")}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	obs := Observer{OnRetry: func(time.Duration, int, int) { cancel() }}
	deps := Deps{Host: h, Provider: fp, Tools: tool.NewRegistry(tool.Builtins()...), Observer: obs}
	_, err := Run(ctx, deps, limitCfg(h))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err %v", err)
	}
	if len(fp.Requests) != 1 || h.Slept != 0 {
		t.Fatalf("requests %d slept %v", len(fp.Requests), h.Slept)
	}
}

func TestPermanentErrorsNotRetried(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{name: "bad request", err: &provider.HTTPError{Status: 400, Body: "bad"}},
		{name: "unauthorized", err: &provider.HTTPError{Status: 401, Body: "no"}},
		{name: "plain", err: errCut},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := host.NewMemory()
			fp := &fake.Provider{Turns: []fake.Turn{{Err: tc.err}, fake.TextTurn("never")}}
			deps := Deps{Host: h, Provider: fp, Tools: tool.NewRegistry(tool.Builtins()...)}
			_, err := Run(context.Background(), deps, limitCfg(h))
			if !errors.Is(err, tc.err) {
				t.Fatalf("err %v", err)
			}
			if len(fp.Requests) != 1 || h.Slept != 0 {
				t.Fatalf("requests %d slept %v", len(fp.Requests), h.Slept)
			}
		})
	}
}

func TestTransientStatusUsesAllAttempts(t *testing.T) {
	cause := &provider.HTTPError{Status: 502, Body: "gateway"}
	turns := make([]fake.Turn, 0, maxStreamAttempts)
	for i := 0; i < maxStreamAttempts; i++ {
		turns = append(turns, fake.Turn{Err: &provider.TransientError{Err: cause}})
	}
	h := host.NewMemory()
	fp := &fake.Provider{Turns: turns}
	deps := Deps{Host: h, Provider: fp, Tools: tool.NewRegistry(tool.Builtins()...)}
	_, err := Run(context.Background(), deps, limitCfg(h))
	if !errors.Is(err, cause) || len(fp.Requests) != maxStreamAttempts {
		t.Fatalf("err %v requests %d", err, len(fp.Requests))
	}
}

func TestRetriesLeaveLimitBackoffAlone(t *testing.T) {
	h := host.NewMemory()
	fp := &fake.Provider{Turns: []fake.Turn{
		partialTurn(),
		partialTurn(),
		{Err: &provider.RateLimitError{}},
		fake.TextTurn("ok"),
	}}
	deps := Deps{Host: h, Provider: fp, Tools: tool.NewRegistry(tool.Builtins()...)}
	if _, err := Run(context.Background(), deps, limitCfg(h)); err != nil {
		t.Fatal(err)
	}
	if h.Slept != 8*time.Second {
		t.Fatalf("slept %v", h.Slept)
	}
}
