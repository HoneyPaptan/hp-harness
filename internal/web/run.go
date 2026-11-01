package web

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/HoneyPaptan/hp-harness/internal/core"
	"github.com/HoneyPaptan/hp-harness/internal/journal"
	"github.com/HoneyPaptan/hp-harness/internal/provider"
)

var errActive = errors.New("a turn is active on this run")

type subscriber struct {
	ch      chan Event
	dropped chan struct{}
}

type run struct {
	id     string
	b      Backend
	queue  int
	mu     sync.Mutex
	log    eventLog
	subs   map[*subscriber]struct{}
	seen   int
	status string
	busy   bool
	cancel context.CancelFunc
	done   chan struct{}
	musing bool
}

func newRun(id string, b Backend, queue int) *run {
	return &run{id: id, b: b, queue: queue, subs: make(map[*subscriber]struct{})}
}

func (r *run) emit(ev Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.publish(ev)
}

func (r *run) publish(ev Event) {
	ev = r.log.add(ev)
	for s := range r.subs {
		select {
		case s.ch <- ev:
		default:
			delete(r.subs, s)
			close(s.dropped)
		}
	}
}

func (r *run) subscribe(after int64) ([]Event, *subscriber) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s := &subscriber{ch: make(chan Event, r.queue), dropped: make(chan struct{})}
	r.subs[s] = struct{}{}
	return r.log.since(after), s
}

func (r *run) unsubscribe(s *subscriber) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.subs, s)
}

func (r *run) load(recs []journal.Record) {
	for _, rec := range recs {
		for _, ev := range recordEvents(rec) {
			if ev.Kind != KindText || ev.Text != "" {
				r.publish(ev)
			}
		}
	}
	r.seen += len(recs)
}

func (r *run) refresh() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.refreshLocked()
}

func (r *run) refreshLocked() {
	if r.busy {
		return
	}
	recs, err := r.b.Records(r.id)
	if err != nil {
		return
	}
	r.load(recs[min(r.seen, len(recs)):])
	r.setStatus(r.idleStatus(recs))
}

func (r *run) idleStatus(recs []journal.Record) string {
	if r.b.Busy(r.id) {
		return StatusRunning
	}
	st, err := journal.Replay(recs)
	if err == nil && st.Finished {
		return StatusFinished
	}
	return StatusInterrupted
}

func (r *run) setStatus(status string) {
	if r.status == status {
		return
	}
	r.status = status
	r.publish(Event{Kind: KindRunStatus, Status: status})
}

func (r *run) reserve() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.busy {
		return errActive
	}
	r.refreshLocked()
	r.busy = true
	return nil
}

func (r *run) unreserve() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.busy = false
}

func (r *run) begin(prompt string, cancel context.CancelFunc) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if prompt != "" {
		r.publish(Event{Kind: KindUser, Text: prompt})
	}
	r.cancel, r.done, r.musing = cancel, make(chan struct{}), false
	r.setStatus(StatusRunning)
}

func (r *run) finish(ctx context.Context, s Stats, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	status := r.conclude(ctx, s, err)
	r.syncSeen()
	r.setStatus(status)
	r.busy, r.cancel = false, nil
	close(r.done)
}

func (r *run) conclude(ctx context.Context, s Stats, err error) string {
	if err == nil {
		r.publish(turnEndEvent(s))
		return StatusFinished
	}
	if !errors.Is(err, context.Canceled) && ctx.Err() == nil {
		r.publish(Event{Kind: KindError, Text: err.Error()})
	}
	return StatusInterrupted
}

func (r *run) syncSeen() {
	if recs, err := r.b.Records(r.id); err == nil {
		r.seen = len(recs)
	}
}

func (r *run) stop(wait time.Duration) bool {
	r.mu.Lock()
	cancel, done := r.cancel, r.done
	r.mu.Unlock()
	if cancel == nil {
		return false
	}
	cancel()
	select {
	case <-done:
	case <-time.After(wait):
	}
	return true
}

func (r *run) failedAttempt(ev Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.musing = false
	ev.Discard = r.log.dropPartial()
	r.publish(ev)
}

func (r *run) toolStart(c provider.ToolCall) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.musing = false
	r.publish(toolStartEvent(c))
}

func (r *run) streamEvent(ev provider.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if ev.Kind == provider.EventReasoning {
		r.startMusing()
		return
	}
	if ev.Text != "" {
		r.musing = false
		r.publish(Event{Kind: KindText, Text: ev.Text})
	}
}

func (r *run) startMusing() {
	if !r.musing {
		r.musing = true
		r.publish(Event{Kind: KindThinking})
	}
}

func (r *run) observer() core.Observer {
	return core.Observer{
		OnEvent:      r.streamEvent,
		OnToolStart:  r.toolStart,
		OnToolResult: func(tr provider.ToolResult) { r.emit(toolResultEvent(tr)) },
		OnWait: func(spec string, d time.Duration, until time.Time) {
			r.failedAttempt(Event{Kind: KindWait, Model: spec, Until: until, DelayMS: d.Milliseconds()})
		},
		OnSwitch: func(from, to string) {
			r.failedAttempt(Event{Kind: KindSwitch, From: from, To: to})
		},
		OnRetry: func(d time.Duration, attempt, limit int) {
			r.failedAttempt(Event{Kind: KindRetry, DelayMS: d.Milliseconds(), Attempt: attempt, Max: limit})
		},
	}
}
