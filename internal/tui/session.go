package tui

import (
	"context"
	"io"
	"time"
)

const (
	promptGlyph = "› "
	flushDelay  = 30 * time.Millisecond
	rewarmAfter = 60 * time.Second
	quitGrace   = 500 * time.Millisecond
	readSize    = 4096
)

type Config struct {
	In         io.Reader
	Out        io.Writer
	Width      func() int
	Color      bool
	Agent      Agent
	Header     string
	Resume     bool
	RunChoices []RunChoice
	OpenRun    func(id string) (Agent, bool, string, error)
	Intro      string
	PickModel  bool
	Now        func() time.Time
	CrewPoll   time.Duration
	CrewFollow time.Duration
}

type turnResult struct {
	outcome  Outcome
	err      error
	panicked any
}

type turnFunc func(context.Context, func(Event)) (Outcome, error)

type Session struct {
	cfg       Config
	editor    Editor
	picker    *picker
	runPick   *menu
	runIDs    map[string]string
	decoder   Decoder
	renderer  *Renderer
	out       output
	input     chan []byte
	events    chan Event
	done      chan turnResult
	models    chan modelsResult
	stopped   chan struct{}
	flush     <-chan time.Time
	cancel    context.CancelFunc
	lastKey   time.Time
	keyName   string
	stopping  bool
	masking   bool
	armed     bool
	drawn     bool
	dirty     bool
	quit      bool
	pending   string
	queue     []string
	complete  *completion
	fileCache []string
	approval  *approval
	crew      *crewFlow
	drafts    chan draftResult
	crewID    string
	crewBrief string
	crewTick  <-chan time.Time
	view      *crewView
	viewTick  <-chan time.Time
}

func NewSession(cfg Config) *Session {
	renderer := NewRenderer(cfg.Out, cfg.Width())
	renderer.SetColor(cfg.Color)
	return &Session{
		cfg:      withClock(cfg),
		renderer: renderer,
		out:      output{w: cfg.Out, color: cfg.Color},
		input:    make(chan []byte),
		events:   make(chan Event),
		done:     make(chan turnResult),
		models:   make(chan modelsResult, 1),
		drafts:   make(chan draftResult, 1),
		stopped:  make(chan struct{}),
	}
}

func withClock(cfg Config) Config {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.CrewPoll <= 0 {
		cfg.CrewPoll = crewPollDefault
	}
	if cfg.CrewFollow <= 0 {
		cfg.CrewFollow = crewFollowDefault
	}
	return cfg
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

func (s *Session) preconnect(ctx context.Context) {
	s.lastKey = s.cfg.Now()
	if p, ok := s.cfg.Agent.(Preconnector); ok {
		p.Preconnect(ctx)
	}
}

func (s *Session) rewarm(ctx context.Context) {
	if !s.busy() && s.cfg.Now().Sub(s.lastKey) >= rewarmAfter {
		s.preconnect(ctx)
	}
	s.lastKey = s.cfg.Now()
}

func (s *Session) begin(ctx context.Context) {
	s.preconnect(ctx)
	s.greet()
	if len(s.cfg.RunChoices) > 0 {
		s.startRunPick()
		s.dirty = true
		s.redraw()
		return
	}
	if s.cfg.Resume {
		s.startTurn(ctx, s.cfg.Agent.Resume)
		return
	}
	if s.cfg.PickModel {
		s.startPicker()
	}
	s.dirty = true
	s.redraw()
}

func (s *Session) greet() {
	if s.cfg.Header == "" && s.cfg.Intro == "" {
		return
	}
	if s.cfg.Header != "" {
		s.note(s.cfg.Header)
	}
	if s.cfg.Intro != "" {
		s.note(s.cfg.Intro)
	}
	s.out.gap()
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
	case res := <-s.models:
		s.handleModelsResult(res)
	case res := <-s.drafts:
		s.handleDraft(res)
	case <-s.crewTick:
		s.pollCrew()
	case <-s.viewTick:
		s.pollView()
	}
}

func (s *Session) feed(ctx context.Context, chunk []byte) {
	if chunk == nil {
		s.stopNow()
		return
	}
	s.rewarm(ctx)
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
	if ev.Kind == EventApprove {
		if ev.Reply != nil && !s.stopping {
			s.openApproval(ev)
		}
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
	s.out.setSuffix("")
	s.lastKey = s.cfg.Now()
	interrupted := s.stopping
	if interrupted {
		s.stopping = false
	}
	switch {
	case ctx.Err() != nil && !interrupted:
		s.out.gap()
		s.dirty = true
		return
	case res.err != nil:
		if IsNoModel(res.err) {
			s.pickModelFirst("")
		} else if !interrupted {
			s.fail(res.err)
		}
	default:
		if !interrupted {
			s.note(statusLine(res.outcome))
		}
	}
	s.out.gap()
	s.dirty = true
	s.startQueued(ctx)
}

func (s *Session) startQueued(ctx context.Context) {
	for len(s.queue) > 0 && !s.busy() && s.idle() {
		text := s.queue[0]
		s.queue = s.queue[1:]
		s.note(promptGlyph + firstLine(text))
		s.startPrompt(ctx, text)
	}
	if s.busy() {
		s.refreshBusyLine()
		return
	}
	s.out.setSuffix("")
}

func (s *Session) idle() bool {
	return s.picker == nil && s.view == nil && s.crew == nil && !s.masking && s.approval == nil
}

func (s *Session) settle(res turnResult) {
	s.dropApproval()
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
	s.noteQuit()
}

func (s *Session) noteQuit() {
	noter, ok := s.cfg.Agent.(QuitNoter)
	if !ok {
		return
	}
	for _, line := range noter.QuitNotes() {
		s.note(line)
	}
}

func (s *Session) note(text string) {
	s.out.line(sgrDim, text)
}

func (s *Session) fail(err error) {
	s.out.line(sgrRed, shortError(err))
}

func (s *Session) redraw() {
	if s.approval != nil {
		s.drawApproval()
		return
	}
	if !s.dirty || s.quit || s.busy() {
		return
	}
	s.dirty = false
	s.drawn = true
	s.renderer.SetWidth(s.cfg.Width())
	if s.runPick != nil {
		rows, sel := s.runPick.window()
		_ = s.renderer.DrawMenu(runPrompt, s.editor.Runes(), s.editor.Cursor(), rows, sel, "", runPickMore(s.runPick), runFooter)
		return
	}
	if s.picker != nil && s.picker.stage == pickProvider {
		rows, sel := s.picker.menu.window()
		_ = s.renderer.DrawMenu(providerPrompt, s.editor.Runes(), s.editor.Cursor(), rows, sel, "", pickerMore(s.picker.menu), s.footer())
		return
	}
	if s.picker != nil && s.picker.stage == pickModel {
		rows, sel := s.picker.menu.window()
		_ = s.renderer.DrawMenu(modelPrompt(s.picker.provider), s.editor.Runes(), s.editor.Cursor(), markFree(rows), sel, s.picker.modelsNote, modelPickerMore(s.picker), s.footer())
		return
	}
	if s.view != nil {
		s.drawView()
		return
	}
	if s.crew != nil {
		_ = s.renderer.Draw(s.crewPrompt(), s.editor.Runes(), s.editor.Cursor(), s.footer())
		return
	}
	if s.masking {
		_ = s.renderer.DrawMasked(s.prompt(), s.editor.Runes(), s.editor.Cursor(), s.footer())
		return
	}
	if s.complete != nil && s.complete.visible() {
		rows, sel := s.complete.menu.window()
		_ = s.renderer.DrawMenu(promptGlyph, s.editor.Runes(), s.editor.Cursor(), rows, sel, "", completionMore(s.complete.menu), s.footer())
		return
	}
	_ = s.renderer.Draw(s.prompt(), s.editor.Runes(), s.editor.Cursor(), s.footer())
}

func (s *Session) prompt() string {
	if s.picker != nil {
		if s.picker.stage == pickProvider {
			return providerPrompt
		}
		return "model for " + s.picker.provider + ": "
	}
	if s.masking {
		return "key " + s.keyName + ": "
	}
	return promptGlyph
}

func footerLine(a Agent) string {
	mode := a.PermissionMode()
	if mode == "" {
		mode = "auto"
	}
	if a.Model() == "" {
		return mode + " · no model · /model"
	}
	line := mode + " · " + a.Model() + " · jev"
	if !a.Jev() {
		line += " off"
	}
	return line
}

func (s *Session) endPrompt() {
	if s.picker != nil || s.runPick != nil {
		s.clearDrawn()
		return
	}
	if !s.drawn {
		return
	}
	s.drawn = false
	_ = s.renderer.Finish()
}

func (s *Session) clearDrawn() {
	if !s.drawn {
		return
	}
	s.drawn = false
	_ = s.renderer.Clear()
}
