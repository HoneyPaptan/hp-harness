package tui

import (
	"context"
	"strings"
)

func (s *Session) handleKeys(ctx context.Context, keys []Key) {
	for _, k := range keys {
		if s.quit {
			return
		}
		s.handleKey(ctx, k)
	}
}

func (s *Session) handleKey(ctx context.Context, k Key) {
	if s.busy() {
		s.turnKey(k)
		return
	}
	if k.Type != KeyCtrlC {
		s.armed = false
	}
	s.promptKey(ctx, k)
}

func (s *Session) turnKey(k Key) {
	switch k.Type {
	case KeyCtrlC, KeyEsc:
		s.interrupt()
	case KeyEnter, KeyCtrlD, KeyCtrlL:
	default:
		s.editor.Handle(k)
	}
}

func (s *Session) promptKey(ctx context.Context, k Key) {
	switch s.editor.Handle(k) {
	case ActionSubmit:
		s.submit(ctx)
	case ActionInterrupt:
		s.promptInterrupt()
	case ActionEOF:
		s.quit = true
	default:
		s.dirty = true
	}
}

func (s *Session) promptInterrupt() {
	s.dirty = true
	if s.editor.Text() != "" {
		s.editor.Clear()
		s.armed = false
		return
	}
	if s.armed {
		s.quit = true
		return
	}
	s.armed = true
	s.drawn = false
	_ = s.renderer.Clear()
	s.note(quitHint)
}

func (s *Session) submit(ctx context.Context) {
	if strings.TrimSpace(s.editor.Text()) == "" {
		s.editor.Clear()
		s.dirty = true
		return
	}
	s.dirty = true
	s.redraw()
	text := s.editor.Take()
	s.endPrompt()
	if name, arg, ok := splitSlash(text); ok {
		s.runSlash(name, arg)
		s.dirty = true
		return
	}
	prompt := promptText(text)
	s.startTurn(ctx, func(ctx context.Context, emit func(Event)) (Outcome, error) {
		return s.cfg.Agent.Send(ctx, prompt, emit)
	})
}
