package cli

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"

	"github.com/HoneyPaptan/hp-harness/internal/host"
	"github.com/HoneyPaptan/hp-harness/internal/permit"
	"github.com/HoneyPaptan/hp-harness/internal/provider"
)

func TestGateDenyHold(t *testing.T) {
	h := host.NewMemory()
	mode := permit.ModeAuto
	gate := newGate(h, func() permit.Mode { return mode }, []permit.Rule{
		{Tool: "bash", Pattern: "git push *", Verdict: permit.Deny},
	}, &permit.Grants{}, "/w", "/home/u", nil, nil)
	call := provider.ToolCall{ID: "1", Name: "bash", Args: json.RawMessage(`{"command":"git push origin main"}`)}
	v := gate(context.Background(), call)
	if v.Allow || v.Reason != "denied by rule bash git push *" {
		t.Fatalf("verdict %+v", v)
	}
}

func TestGateAskNilHold(t *testing.T) {
	h := host.NewMemory()
	mode := permit.ModeAsk
	gate := newGate(h, func() permit.Mode { return mode }, nil, &permit.Grants{}, "/w", "/home/u", nil, nil)
	call := provider.ToolCall{ID: "1", Name: "bash", Args: json.RawMessage(`{"command":"git push origin main"}`)}
	v := gate(context.Background(), call)
	want := "needs approval: run it in a session or use --full-access"
	if v.Allow || v.Reason != want {
		t.Fatalf("verdict %+v", v)
	}
}

func TestGateAskAlwaysGrant(t *testing.T) {
	h := host.NewMemory()
	mode := permit.ModeAsk
	grants := &permit.Grants{}
	var once atomic.Bool
	ask := func(context.Context, permit.Action) askAnswer {
		if once.CompareAndSwap(false, true) {
			return askAlways
		}
		return askYes
	}
	gate := newGate(h, func() permit.Mode { return mode }, nil, grants, "/w", "/home/u", ask, nil)
	call := provider.ToolCall{ID: "1", Name: "bash", Args: json.RawMessage(`{"command":"git push origin main"}`)}
	v := gate(context.Background(), call)
	if !v.Allow {
		t.Fatalf("first %+v", v)
	}
	v = gate(context.Background(), call)
	if !v.Allow {
		t.Fatalf("second %+v", v)
	}
}

func TestGateAskNoHold(t *testing.T) {
	h := host.NewMemory()
	mode := permit.ModeAsk
	ask := func(context.Context, permit.Action) askAnswer { return askNo }
	gate := newGate(h, func() permit.Mode { return mode }, nil, &permit.Grants{}, "/w", "/home/u", ask, nil)
	call := provider.ToolCall{ID: "1", Name: "bash", Args: json.RawMessage(`{"command":"git push origin main"}`)}
	v := gate(context.Background(), call)
	if v.Allow || v.Reason != "declined by the user" {
		t.Fatalf("verdict %+v", v)
	}
}

func TestGateReviewHold(t *testing.T) {
	h := host.NewMemory()
	mode := permit.ModeAuto
	gate := newGate(h, func() permit.Mode { return mode }, nil, &permit.Grants{}, "/w", "/home/u", nil, nil)
	call := provider.ToolCall{ID: "1", Name: "bash", Args: json.RawMessage(`{"command":"git push origin main"}`)}
	v := gate(context.Background(), call)
	if v.Allow || v.Reason != "needs review" {
		t.Fatalf("verdict %+v", v)
	}
}

func TestGateModeSwitch(t *testing.T) {
	h := host.NewMemory()
	mode := permit.ModeAuto
	gate := newGate(h, func() permit.Mode { return mode }, nil, &permit.Grants{}, "/w", "/home/u", nil, nil)
	call := provider.ToolCall{ID: "1", Name: "bash", Args: json.RawMessage(`{"command":"git push origin main"}`)}
	if v := gate(context.Background(), call); v.Allow || v.Reason != "needs review" {
		t.Fatalf("auto review %+v", v)
	}
	mode = permit.ModeFull
	if v := gate(context.Background(), call); !v.Allow {
		t.Fatalf("full %+v", v)
	}
}

func TestGateWriteSymlinkReviewed(t *testing.T) {
	h := host.NewMemory()
	h.Link("/w/link", "/else")
	mode := permit.ModeAuto
	gate := newGate(h, func() permit.Mode { return mode }, nil, &permit.Grants{}, "/w", "/home/u", nil, nil)
	call := provider.ToolCall{ID: "1", Name: "write", Args: json.RawMessage(`{"path":"link/new.go"}`)}
	v := gate(context.Background(), call)
	if v.Allow || v.Reason != "needs review" {
		t.Fatalf("verdict %+v", v)
	}
}

func TestGateWriteNewFileRoutine(t *testing.T) {
	h := host.NewMemory()
	mode := permit.ModeAuto
	gate := newGate(h, func() permit.Mode { return mode }, nil, &permit.Grants{}, "/w", "/home/u", nil, nil)
	call := provider.ToolCall{ID: "1", Name: "write", Args: json.RawMessage(`{"path":"new.go","contents":"x"}`)}
	v := gate(context.Background(), call)
	if !v.Allow {
		t.Fatalf("verdict %+v", v)
	}
}

func TestGateWriteDotDotThroughLinkReviewed(t *testing.T) {
	h := host.NewMemory()
	h.Link("/w/link", "/else/deep")
	mode := permit.ModeAuto
	gate := newGate(h, func() permit.Mode { return mode }, nil, &permit.Grants{}, "/w", "/home/u", nil, nil)
	call := provider.ToolCall{ID: "1", Name: "write", Args: json.RawMessage(`{"path":"link/../x.go"}`)}
	v := gate(context.Background(), call)
	if v.Allow {
		t.Fatalf("verdict %+v", v)
	}
}
