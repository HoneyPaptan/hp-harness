package web

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/HoneyPaptan/hp-harness/internal/core"
)

const stopWait = 5 * time.Second

var errClosed = errors.New("server is shutting down")

type turnFunc func(ctx context.Context, obs core.Observer) (Stats, error)

type manager struct {
	b      Backend
	queue  int
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	mu     sync.Mutex
	runs   map[string]*run
	closed bool
}

func newManager(b Backend, queue int) *manager {
	ctx, cancel := context.WithCancel(context.Background())
	return &manager{b: b, queue: queue, ctx: ctx, cancel: cancel, runs: make(map[string]*run)}
}

func (m *manager) run(id string) (*run, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if r, ok := m.runs[id]; ok {
		return r, nil
	}
	if _, err := m.b.Records(id); err != nil {
		return nil, err
	}
	r := newRun(id, m.b, m.queue)
	r.refresh()
	m.runs[id] = r
	return r, nil
}

func (m *manager) start(model, prompt string) (string, error) {
	agent, err := m.b.Start(model, prompt)
	if err != nil {
		return "", err
	}
	r := newRun(agent.ID(), m.b, m.queue)
	r.busy = true
	if recs, err := m.b.Records(r.id); err == nil {
		r.load(recs)
	}
	m.mu.Lock()
	m.runs[r.id] = r
	m.mu.Unlock()
	return r.id, m.launch(r, agent, "", agent.Resume)
}

func (m *manager) send(id, prompt string) error {
	r, err := m.run(id)
	if err != nil {
		return err
	}
	if err := r.reserve(); err != nil {
		return err
	}
	agent, err := m.b.Open(id)
	if err != nil {
		r.unreserve()
		return err
	}
	return m.launch(r, agent, prompt, func(ctx context.Context, obs core.Observer) (Stats, error) {
		return agent.Send(ctx, prompt, obs)
	})
}

func (m *manager) launch(r *run, agent Agent, prompt string, turn turnFunc) error {
	if !m.track() {
		agent.Release()
		r.unreserve()
		return errClosed
	}
	ctx, cancel := context.WithCancel(m.ctx)
	r.begin(prompt, cancel)
	go func() {
		defer m.wg.Done()
		defer cancel()
		s, err := turn(ctx, r.observer())
		agent.Release()
		r.finish(ctx, s, err)
	}()
	return nil
}

func (m *manager) track() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return false
	}
	m.wg.Add(1)
	return true
}

func (m *manager) stop(id string) (bool, error) {
	r, err := m.run(id)
	if err != nil {
		return false, err
	}
	if r.stop(stopWait) {
		return true, nil
	}
	if m.b.Busy(id) {
		return false, ErrBusy
	}
	return false, nil
}

func (m *manager) setModel(id, spec string) (string, error) {
	r, err := m.run(id)
	if err != nil {
		return "", err
	}
	if err := r.reserve(); err != nil {
		return "", err
	}
	defer r.unreserve()
	agent, err := m.b.Open(id)
	if err != nil {
		return "", err
	}
	defer agent.Release()
	from := agent.Model()
	if err := agent.SetModel(spec); err != nil {
		return "", err
	}
	to := agent.Model()
	if to != from {
		r.recordSwitch(from, to)
	}
	return to, nil
}

func (r *run) recordSwitch(from, to string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.publish(Event{Kind: KindSwitch, From: from, To: to})
	r.syncSeen()
}

func (m *manager) close() {
	m.mu.Lock()
	m.closed = true
	m.mu.Unlock()
	m.cancel()
	m.wg.Wait()
}
