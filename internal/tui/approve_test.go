package tui

import (
	"context"
	"strings"
	"testing"
	"time"
)

func approveTurn(action string, got chan Approval) func(context.Context, func(Event)) (Outcome, error) {
	return func(_ context.Context, emit func(Event)) (Outcome, error) {
		ch := make(chan Approval, 1)
		emit(Event{Kind: EventApprove, Text: action, Reply: ch})
		got <- <-ch
		return Outcome{Model: "xai/grok", Steps: 1}, nil
	}
}

func TestApproveYesEnter(t *testing.T) {
	got := make(chan Approval, 1)
	h := startSession(t, newFakeAgent(approveTurn("write: .env", got)), false, false)
	h.play([]step{{keys: "hi\r"}, {wait: "write: .env"}, {keys: "\r"}})
	if ans := <-got; ans.Answer != ApproveYes {
		t.Fatalf("answer %v", ans)
	}
	if !strings.Contains(h.out.String(), "write: .env") || !strings.Contains(h.out.String(), "allowed") {
		t.Fatalf("out %q", h.out.String())
	}
}

func TestApproveNoDownDownEnter(t *testing.T) {
	got := make(chan Approval, 1)
	h := startSession(t, newFakeAgent(approveTurn("bash: push", got)), false, false)
	h.play([]step{{keys: "x\r"}, {wait: "bash: push"}, {keys: "\x1b[B\x1b[B\r"}})
	if ans := <-got; ans.Answer != ApproveNo {
		t.Fatalf("answer %v", ans)
	}
	if !strings.Contains(h.out.String(), "declined") {
		t.Fatalf("out %q", h.out.String())
	}
}

func TestApproveAlwaysKey(t *testing.T) {
	got := make(chan Approval, 1)
	h := startSession(t, newFakeAgent(approveTurn("write: x", got)), false, false)
	h.play([]step{{keys: "x\r"}, {wait: "write: x"}, {keys: "a"}})
	if ans := <-got; ans.Answer != ApproveAlways {
		t.Fatalf("answer %v", ans)
	}
	if !strings.Contains(h.out.String(), "allowed for this session") {
		t.Fatalf("out %q", h.out.String())
	}
}

func TestApproveCtrlCSaysNoAndKeepsTheTurn(t *testing.T) {
	got := make(chan Approval, 1)
	h := startSession(t, newFakeAgent(approveTurn("write: x", got)), false, false)
	h.play([]step{{keys: "x\r"}, {wait: "write: x"}, {keys: "\x03"}})
	if ans := <-got; ans.Answer != ApproveNo {
		t.Fatalf("answer %v", ans)
	}
	h.play([]step{{wait: "declined"}})
	if strings.Contains(h.out.String(), "interrupted") {
		t.Fatalf("Ctrl-C stopped the turn: %q", h.out.String())
	}
}

func TestApproveEscSaysNoAndStopsTheTurn(t *testing.T) {
	got := make(chan Approval, 1)
	h := startSession(t, newFakeAgent(approveTurn("write: x", got)), false, false)
	h.play([]step{{keys: "x\r"}, {wait: "write: x"}, {keys: "\x1b"}})
	if ans := <-got; ans.Answer != ApproveNo {
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
			got := make(chan Approval, 1)
			h := startSession(t, newFakeAgent(approveTurn("write: x", got)), false, false)
			h.play([]step{{keys: "x\r"}, {wait: firstLine(approveHint)}, {keys: key}})
			if ans := <-got; ans.Answer != want {
				t.Fatalf("answer %v, want %v", ans, want)
			}
		})
	}
}

func TestApproveIgnoresOtherKeys(t *testing.T) {
	got := make(chan Approval, 1)
	h := startSession(t, newFakeAgent(approveTurn("write: x", got)), false, false)
	h.play([]step{{keys: "x\r"}, {wait: "write: x"}, {keys: "qz\r"}})
	if ans := <-got; ans.Answer != ApproveYes {
		t.Fatalf("answer %v", ans)
	}
}

func TestApproveSingleReply(t *testing.T) {
	extra := make(chan struct{}, 1)
	turn := func(_ context.Context, emit func(Event)) (Outcome, error) {
		ch := make(chan Approval, 1)
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

func TestApproveNoWithANote(t *testing.T) {
	got := make(chan Approval, 1)
	h := startSession(t, newFakeAgent(approveTurn("bash: git push", got)), false, false)
	h.play([]step{{keys: "hi\r"}, {wait: "bash: git push"}, {keys: "\x1b[B\x1b[B\t"}, {wait: noteNoPrompt}, {keys: "use a branch\r"}})
	if ans := <-got; ans.Answer != ApproveNo || ans.Note != "use a branch" {
		t.Fatalf("answer %+v", ans)
	}
	h.play([]step{{wait: "declined: use a branch"}})
}

func TestApproveYesWithANoteQueuesIt(t *testing.T) {
	got := make(chan Approval, 1)
	a := newFakeAgent(approveTurn("write: a.go", got), replyTurn("second turn"))
	h := startSession(t, a, false, false)
	h.play([]step{{keys: "hi\r"}, {wait: "write: a.go"}, {keys: "\t"}, {wait: noteYesPrompt}, {keys: "then run the tests\r"}})
	if ans := <-got; ans.Answer != ApproveYes || ans.Note != "" {
		t.Fatalf("answer %+v", ans)
	}
	h.play([]step{{wait: "second turn"}})
	if prompts := a.sent(); len(prompts) != 2 || prompts[1] != "then run the tests" {
		t.Fatalf("prompts %q", prompts)
	}
}

func TestApproveNoteEscGoesBackToTheChoices(t *testing.T) {
	got := make(chan Approval, 1)
	h := startSession(t, newFakeAgent(approveTurn("write: a.go", got)), false, false)
	h.play([]step{{keys: "hi\r"}, {wait: "write: a.go"}, {keys: "\t"}, {wait: noteYesPrompt}, {keys: "typo\x1b"}, {keys: "3"}})
	if ans := <-got; ans.Answer != ApproveNo || ans.Note != "" {
		t.Fatalf("answer %+v", ans)
	}
}

func TestApproveTabSkipsTheAlwaysChoice(t *testing.T) {
	got := make(chan Approval, 1)
	h := startSession(t, newFakeAgent(approveTurn("write: a.go", got)), false, false)
	h.play([]step{{keys: "hi\r"}, {wait: "write: a.go"}, {keys: "\x1b[B\t\r"}})
	if ans := <-got; ans.Answer != ApproveNo {
		t.Fatalf("answer %+v", ans)
	}
}
