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
	if s.picker != nil {
		s.pickerKey(k)
		return
	}
	if s.masking {
		s.secretKey(k)
		return
	}
	if k.Type != KeyCtrlC {
		s.armed = false
	}
	s.promptKey(ctx, k)
}

func (s *Session) pickerKey(k Key) {
	if s.picker.stage == pickProvider {
		s.providerKey(k)
		return
	}
	s.modelKey(k)
}

func (s *Session) providerKey(k Key) {
	switch k.Type {
	case KeyUp:
		s.picker.move(-1)
		s.dirty = true
	case KeyDown:
		s.picker.move(1)
		s.dirty = true
	case KeyEnter:
		if s.picker.pick() {
			s.editor.Clear()
			s.beginModelFetch()
			s.dirty = true
		}
	case KeyEsc, KeyCtrlC:
		s.cancelPicker()
	default:
		s.editor.Handle(k)
		s.picker.setFilter(s.editor.Text())
		s.dirty = true
	}
}

func (s *Session) modelKey(k Key) {
	switch k.Type {
	case KeyUp:
		s.picker.move(-1)
		s.dirty = true
	case KeyDown:
		s.picker.move(1)
		s.dirty = true
	case KeyEnter:
		s.submitModel()
	case KeyEsc, KeyCtrlC:
		s.cancelPicker()
	default:
		s.editor.Handle(k)
		if !s.picker.loading {
			s.picker.setFilter(s.editor.Text())
		}
		s.dirty = true
	}
}

func (s *Session) submitModel() {
	typed := strings.TrimSpace(s.editor.Text())
	if chosen, ok := s.picker.menu.chosen(); ok {
		spec := s.picker.provider + "/" + chosen
		s.closePicker()
		s.switchModel(spec)
		s.dirty = true
		return
	}
	if typed == "" {
		s.cancelPicker()
		return
	}
	spec := s.picker.provider + "/" + typed
	s.closePicker()
	s.switchModel(spec)
	s.dirty = true
}

func (s *Session) cancelPicker() {
	s.closePicker()
	s.note(pickerNote)
	s.dirty = true
}

func (s *Session) closePicker() {
	s.cancelModelFetch()
	s.picker = nil
	s.editor.Clear()
	s.clearDrawn()
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

func (s *Session) secretKey(k Key) {
	switch k.Type {
	case KeyEnter:
		s.saveKey()
	case KeyEsc, KeyCtrlC:
		s.cancelKey()
	case KeyUp, KeyDown:
	default:
		s.editor.Handle(k)
		s.dirty = true
	}
}

func (s *Session) saveKey() {
	name, value := s.keyName, s.editor.Text()
	s.endKey()
	if value == "" {
		s.note(keyNotSaved)
		return
	}
	if err := s.cfg.Agent.SaveKey(name, value); err != nil {
		s.fail(err)
		return
	}
	s.note("saved " + name)
}

func (s *Session) cancelKey() {
	s.endKey()
	s.note(keyNotSaved)
}

func (s *Session) endKey() {
	s.keyName = ""
	s.masking = false
	s.editor.Clear()
	s.clearDrawn()
	s.dirty = true
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
	s.clearDrawn()
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
