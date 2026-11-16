package tui

import (
	"context"
	"reflect"
	"testing"
)

func gatedTurn(gate chan struct{}) func(context.Context, func(Event)) (Outcome, error) {
	return func(ctx context.Context, emit func(Event)) (Outcome, error) {
		emit(Event{Kind: EventStep})
		<-gate
		return replyTurn("")(ctx, emit)
	}
}

func queueHarness(t *testing.T) (*harness, chan struct{}) {
	t.Helper()
	gate := make(chan struct{})
	h := startSession(t, newFakeAgent(gatedTurn(gate)), false, false)
	h.play([]step{{keys: "first\r"}, {turn: true}})
	return h, gate
}

func finishQueue(t *testing.T, h *harness, gate chan struct{}, followUps int) {
	t.Helper()
	close(gate)
	for range followUps {
		h.waitTurn()
	}
	h.waitForCount("8 in", followUps+1)
	h.play([]step{{keys: "\x04"}})
	h.waitExit()
}

func TestSessionQueuesBehindQueuedMessages(t *testing.T) {
	h, gate := queueHarness(t)
	h.play([]step{{keys: "one\r"}, {wait: "queued 1: one"}, {keys: "two\r"}, {wait: "queued 2: two"}})
	finishQueue(t, h, gate, 2)
	if want := []string{"first", "one", "two"}; !reflect.DeepEqual(h.agent.prompts, want) {
		t.Fatalf("prompts %q want %q", h.agent.prompts, want)
	}
}

func TestSessionUpTakesTheLastQueuedMessageBack(t *testing.T) {
	h, gate := queueHarness(t)
	h.play([]step{
		{keys: "one\r"}, {wait: "queued 1: one"},
		{keys: "two\r"}, {wait: "queued 2: two"},
		{keys: "\x1b[A"}, {wait: "took back: two"}, {wait: "1 queued · › two"},
		{keys: " edited\r"}, {wait: "queued 2: two edited"},
	})
	finishQueue(t, h, gate, 2)
	if want := []string{"first", "one", "two edited"}; !reflect.DeepEqual(h.agent.prompts, want) {
		t.Fatalf("prompts %q want %q", h.agent.prompts, want)
	}
}

func TestSessionBusyLineShowsWhatYouType(t *testing.T) {
	h, gate := queueHarness(t)
	h.play([]step{{keys: "abc"}, {wait: "working… · › abc"}})
	close(gate)
	h.play([]step{{keys: "\x15"}, {wait: "8 in"}, {keys: "\x04"}})
	h.waitExit()
}

func TestSessionSlashCommandWaitsForTheTurn(t *testing.T) {
	h, gate := queueHarness(t)
	h.play([]step{{keys: "/new\r"}, {wait: busySlashHint}})
	close(gate)
	h.play([]step{{wait: "8 in"}, {keys: "\x15\x04"}})
	h.waitExit()
	if h.agent.resets != 0 {
		t.Fatalf("slash command ran during the turn")
	}
}
