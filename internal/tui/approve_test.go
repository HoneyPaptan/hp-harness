package tui

import (
	"context"
	"strings"
	"testing"
	"time"
)

func approveTurn(action string, got chan ApproveAnswer) func(context.Context, func(Event)) (Outcome, error) {
	return func(_ context.Context, emit func(Event)) (Outcome, error) {
		ch := make(chan ApproveAnswer, 1)
		emit(Event{Kind: EventApprove, Text: action, Reply: ch})
		got <- <-ch
		return Outcome{Model: "xai/grok", Steps: 1}, nil
	}
}

func TestApproveYesEnter(t *testing.T) {
	got := make(chan ApproveAnswer, 1)
	h := startSession(t, newFakeAgent(approveTurn("write: .env", got)), false, false)
	h.play([]step{{keys: "hi\r"}, {wait: "write: .env"}, {keys: "\r"}})
	if ans := <-got; ans != ApproveYes {
		t.Fatalf("answer %v", ans)
	}
	if !strings.Contains(h.out.String(), "write: .env") || !strings.Contains(h.out.String(), "allowed") {
		t.Fatalf("out %q", h.out.String())
	}
}

func TestApproveNoDownDownEnter(t *testing.T) {
	got := make(chan ApproveAnswer, 1)
	h := startSession(t, newFakeAgent(approveTurn("bash: push", got)), false, false)
	h.play([]step{{keys: "x\r"}, {wait: "bash: push"}, {keys: "\x1b[B\x1b[B\r"}})
	if ans := <-got; ans != ApproveNo {
		t.Fatalf("answer %v", ans)
	}
	if !strings.Contains(h.out.String(), "declined") {
		t.Fatalf("out %q", h.out.String())
	}
}

func TestApproveAlwaysKey(t *testing.T) {
	got := make(chan ApproveAnswer, 1)
	h := startSession(t, newFakeAgent(approveTurn("write: x", got)), false, false)
	h.play([]step{{keys: "x\r"}, {wait: "write: x"}, {keys: "a"}})
	if ans := <-got; ans != ApproveAlways {
		t.Fatalf("answer %v", ans)
	}
	if !strings.Contains(h.out.String(), "allowed for this session") {
		t.Fatalf("out %q", h.out.String())
	}
}

func TestApproveCtrlCSaysNoAndKeepsTheTurn(t *testing.T) {
	got := make(chan ApproveAnswer, 1)
	h := startSession(t, newFakeAgent(approveTurn("write: x", got)), false, false)
	h.play([]step{{keys: "x\r"}, {wait: "write: x"}, {keys: "\x03"}})
	if ans := <-got; ans != ApproveNo {
		t.Fatalf("answer %v", ans)
	}
	h.play([]step{{wait: "declined"}})
	if strings.Contains(h.out.String(), "interrupted") {
		t.Fatalf("Ctrl-C stopped the turn: %q", h.out.String())
	}
}

func TestApproveEscSaysNoAndStopsTheTurn(t *testing.T) {
	got := make(chan ApproveAnswer, 1)
	h := startSession(t, newFakeAgent(approveTurn("write: x", got)), false, false)
	h.play([]step{{keys: "x\r"}, {wait: "write: x"}, {keys: "\x1b"}})
	if ans := <-got; ans != ApproveNo {
		t.Fatalf("answer %v", ans)
	}
	if !strings.Contains(h.out.String(), "interrupted") || !strings.Contains(h.out.String(), "declined") {
		t.Fatalf("out %q", h.out.String())
	}
}

func TestApproveNumberKeys(t *testing.T) {
	cases := map[string]ApproveAnswer{"1": ApproveYes, "2": ApproveAlways, "3": ApproveNo}
	for key, want := range cases {
		t.Run(key, func(t *testing.T) {
			got := make(chan ApproveAnswer, 1)
			h := startSession(t, newFakeAgent(approveTurn("write: x", got)), false, false)
			h.play([]step{{keys: "x\r"}, {wait: approveHint}, {keys: key}})
			if ans := <-got; ans != want {
				t.Fatalf("answer %v, want %v", ans, want)
			}
		})
	}
}

func TestApproveIgnoresOtherKeys(t *testing.T) {
	got := make(chan ApproveAnswer, 1)
	h := startSession(t, newFakeAgent(approveTurn("write: x", got)), false, false)
	h.play([]step{{keys: "x\r"}, {wait: "write: x"}, {keys: "qz\r"}})
	if ans := <-got; ans != ApproveYes {
		t.Fatalf("answer %v", ans)
	}
}

func TestApproveSingleReply(t *testing.T) {
	extra := make(chan struct{}, 1)
	turn := func(_ context.Context, emit func(Event)) (Outcome, error) {
		ch := make(chan ApproveAnswer, 1)
		emit(Event{Kind: EventApprove, Text: "write: x", Reply: ch})
		<-ch
		select {
		case <-ch:
			extra <- struct{}{}
		default:
		}
		return Outcome{Model: "xai/grok", Steps: 1}, nil
	}
	h := startSession(t, newFakeAgent(turn), false, false)
	h.play([]step{{keys: "x\r"}, {wait: "write: x"}, {keys: "\r\r"}})
	select {
	case <-extra:
		t.Fatal("second reply on channel")
	case <-time.After(50 * time.Millisecond):
	}
}
