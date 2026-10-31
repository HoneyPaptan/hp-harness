package tui

import (
	"context"
	"io"
	"time"
)

const (
	promptGlyph = "› "
	flushDelay  = 30 * time.Millisecond
	quitGrace   = 500 * time.Millisecond
	readSize    = 4096
)

type Config struct {
	In     io.Reader
	Out    io.Writer
	Width  func() int
	Color  bool
	Agent  Agent
	Resume bool
	Intro  string
}

type turnResult struct {
	outcome  Outcome
	err      error
	panicked any
}

type turnFunc func(context.Context, func(Event)) (Outcome, error)

type Session struct {
	cfg      Config
	editor   Editor
	decoder  Decoder
	renderer *Renderer
	out      output
	input    chan []byte
	events   chan Event
	done     chan turnResult
	stopped  chan struct{}
	flush    <-chan time.Time
	cancel   context.CancelFunc
	stopping bool
	armed    bool
	drawn    bool
	dirty    bool
	quit     bool
}

func NewSession(cfg Config) *Session {
	return &Session{
		cfg:      cfg,
		renderer: NewRenderer(cfg.Out, cfg.Width()),
		out:      output{w: cfg.Out, color: cfg.Color},
		input:    make(chan []byte),
		events:   make(chan Event),
		done:     make(chan turnResult),
		stopped:  make(chan struct{}),
	}
}

func (s *Session) Run(ctx context.Context) {
	defer close(s.stopped)
	go s.readInput()
	s.begin(ctx)
	for !s.quit {
		s.step(ctx)
		s.redraw()
	}
	s.leave()
}

func (s *Session) begin(ctx context.Context) {
	if s.cfg.Intro != "" {
		s.note(s.cfg.Intro)
	}
	if s.cfg.Resume {
		s.startTurn(ctx, s.cfg.Agent.Resume)
		return
	}
	s.dirty = true
	s.redraw()
}

func (s *Session) readInput() {
	for {
		buf := make([]byte, readSize)
		n, err := s.cfg.In.Read(buf)
		if n > 0 && !s.deliver(buf[:n]) {
			return
		}
		if err != nil {
			s.deliver(nil)
			return
		}
	}
}

func (s *Session) deliver(chunk []byte) bool {
	select {
	case s.input <- chunk:
		return true
	case <-s.stopped:
		return false
	}
}

func (s *Session) step(ctx context.Context) {
	select {
	case <-ctx.Done():
		s.stopNow()
	case chunk := <-s.input:
		s.feed(ctx, chunk)
	case <-s.flush:
		s.flush = nil
		s.handleKeys(ctx, s.decoder.Flush())
	case ev := <-s.events:
		s.showEvent(ev)
	case res := <-s.done:
		s.finishTurn(ctx, res)
	}
}

func (s *Session) feed(ctx context.Context, chunk []byte) {
	if chunk == nil {
		s.stopNow()
		return
	}
	s.handleKeys(ctx, s.decoder.Feed(chunk))
	s.flush = nil
	if s.decoder.Waiting() {
		s.flush = time.After(flushDelay)
	}
}

func (s *Session) showEvent(ev Event) {
	if ev.Kind == EventStep {
		s.out.activity()
		return
	}
	if !s.stopping {
		s.out.apply(ev)
	}
}

func (s *Session) busy() bool {
	return s.cancel != nil
}

func (s *Session) startTurn(ctx context.Context, run turnFunc) {
	turnCtx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	s.out.activity()
	go s.runTurn(turnCtx, run)
}

func (s *Session) runTurn(ctx context.Context, run turnFunc) {
	var res turnResult
	defer func() {
		res.panicked = recover()
		select {
		case s.done <- res:
		case <-s.stopped:
		}
	}()
	res.outcome, res.err = run(ctx, s.emit)
}

func (s *Session) emit(ev Event) {
	select {
	case s.events <- ev:
	case <-s.stopped:
	}
}

func (s *Session) finishTurn(ctx context.Context, res turnResult) {
	s.settle(res)
	switch {
	case s.stopping:
		s.stopping = false
	case ctx.Err() != nil:
		return
	case res.err != nil:
		s.fail(res.err)
	default:
		s.note(statusLine(res.outcome))
	}
	s.out.gap()
	s.dirty = true
}

func (s *Session) settle(res turnResult) {
	s.cancel()
	s.cancel = nil
	if res.panicked != nil {
		panic(res.panicked)
	}
}

func (s *Session) interrupt() {
	if s.stopping {
		s.stopNow()
		return
	}
	s.stopping = true
	s.cancel()
	s.note("interrupted")
}

func (s *Session) stopNow() {
	s.quit = true
	if !s.busy() {
		return
	}
	s.cancel()
	timeout := time.After(quitGrace)
	for s.busy() {
		select {
		case <-s.events:
		case res := <-s.done:
			s.settle(res)
		case <-timeout:
			s.cancel = nil
		}
	}
}

func (s *Session) leave() {
	s.endPrompt()
	s.out.endLine()
}

func (s *Session) note(text string) {
	s.out.line(sgrDim, text)
}

func (s *Session) fail(err error) {
	s.out.line(sgrRed, shortError(err))
}

func (s *Session) redraw() {
	if !s.dirty || s.quit || s.busy() {
		return
	}
	s.dirty = false
	s.drawn = true
	s.renderer.SetWidth(s.cfg.Width())
	_ = s.renderer.Draw(promptGlyph, s.editor.Runes(), s.editor.Cursor())
}

func (s *Session) endPrompt() {
	if !s.drawn {
		return
	}
	s.drawn = false
	_ = s.renderer.Finish()
}
