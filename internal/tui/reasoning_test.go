package tui

import (
	"context"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestOutputActivityLabels(t *testing.T) {
	cases := []struct {
		name   string
		events []Event
		want   string
	}{
		{name: "thinking replaces working", events: []Event{{Kind: EventStep}, {Kind: EventThinking}}, want: activityText + eraseLine + thinkingText},
		{name: "repeats print once", events: []Event{{Kind: EventThinking}, {Kind: EventThinking}}, want: thinkingText},
		{name: "answer clears thinking", events: []Event{{Kind: EventThinking}, {Kind: EventText, Text: "hi"}}, want: thinkingText + eraseLine + "hi"},
		{name: "mid line stays quiet", events: []Event{{Kind: EventText, Text: "hi"}, {Kind: EventThinking}}, want: "hi"},
		{name: "next step goes back to working", events: []Event{{Kind: EventThinking}, {Kind: EventStep}}, want: thinkingText + eraseLine + activityText},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var b strings.Builder
			o := output{w: &b}
			for _, ev := range tc.events {
				if ev.Kind == EventStep {
					o.activity()
					continue
				}
				o.apply(ev)
			}
			if b.String() != tc.want {
				t.Fatalf("got %q, want %q", b.String(), tc.want)
			}
		})
	}
}

type warmAgent struct {
	*fakeAgent
	warmed chan struct{}
}

func (a *warmAgent) Preconnect(context.Context) {
	a.warmed <- struct{}{}
}

type stepClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *stepClock) read() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *stepClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

func runWarm(t *testing.T, a *warmAgent, clock *stepClock) *harness {
	t.Helper()
	r, w := io.Pipe()
	h := &harness{t: t, agent: a.fakeAgent, out: &screen{}, writer: w, exited: make(chan struct{})}
	s := NewSession(Config{
		In: r, Out: h.out, Width: func() int { return 80 }, Agent: a, Now: clock.read,
	})
	go func() {
		defer close(h.exited)
		s.Run(context.Background())
	}()
	t.Cleanup(func() { w.Close() })
	return h
}

func expectWarm(t *testing.T, a *warmAgent, want bool) {
	t.Helper()
	wait := 50 * time.Millisecond
	if want {
		wait = waitLimit
	}
	select {
	case <-a.warmed:
		if !want {
			t.Fatal("unexpected preconnect")
		}
	case <-time.After(wait):
		if want {
			t.Fatal("no preconnect")
		}
	}
}

func TestSessionPreconnects(t *testing.T) {
	cases := []struct {
		name string
		idle time.Duration
		want bool
	}{
		{name: "quick key", idle: 59 * time.Second},
		{name: "after idle", idle: 60 * time.Second, want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			clock := &stepClock{now: time.Unix(1000, 0)}
			a := &warmAgent{fakeAgent: newFakeAgent(), warmed: make(chan struct{}, 4)}
			h := runWarm(t, a, clock)
			expectWarm(t, a, true)
			clock.advance(tc.idle)
			h.play([]step{{keys: "a"}})
			expectWarm(t, a, tc.want)
			h.play([]step{{keys: "b"}})
			expectWarm(t, a, false)
		})
	}
}

func TestSessionDoesNotPreconnectDuringATurn(t *testing.T) {
	clock := &stepClock{now: time.Unix(1000, 0)}
	gate := make(chan struct{})
	a := &warmAgent{fakeAgent: newFakeAgent(func(context.Context, func(Event)) (Outcome, error) {
		<-gate
		return Outcome{Model: "m"}, nil
	}), warmed: make(chan struct{}, 4)}
	h := runWarm(t, a, clock)
	expectWarm(t, a, true)
	h.play([]step{{keys: "go\r"}})
	h.waitTurn()
	clock.advance(2 * time.Minute)
	h.play([]step{{keys: "x"}})
	expectWarm(t, a, false)
	close(gate)
	h.play([]step{{wait: "m  steps"}})
	expectWarm(t, a, false)
}
