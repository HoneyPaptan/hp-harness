package tui

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
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
	if k.Type == KeyCtrlZ {
		s.suspend()
		return
	}
	if k.Type != KeyEsc {
		s.escArmed = false
	}
	if s.runPick != nil {
		s.runPickKey(ctx, k)
		return
	}
	if s.busy() {
		if s.approval != nil {
			s.approvalKey(k)
			return
		}
		s.turnKey(ctx, k)
		return
	}
	if s.picker != nil {
		s.pickerKey(k)
		return
	}
	if s.view != nil {
		s.viewKey(k)
		return
	}
	if s.crew != nil {
		s.crewKey(k)
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
	s.showArmedHint()
}

func (s *Session) showArmedHint() {
	hint := ""
	switch {
	case s.armed:
		hint = quitHint
	case s.escArmed:
		hint = escClearHint
	case s.copied:
		hint = copiedHint
	}
	s.copied = false
	if hint != s.hint {
		s.hint = hint
		s.dirty = true
	}
}

func (s *Session) suspend() {
	if s.cfg.Suspend == nil {
		return
	}
	if !s.busy() {
		s.clearDrawn()
	}
	s.out.endLine()
	s.cfg.Suspend()
	s.dirty = true
	if s.busy() {
		s.out.restoreActivity()
	}
}

func (s *Session) pickerKey(k Key) {
	if s.picker.stage == pickProvider {
		s.providerKey(k)
		return
	}
	s.modelKey(k)
}

func (s *Session) providerKey(k Key) {
	if s.picker.menu.step(k) {
		s.dirty = true
		return
	}
	switch k.Type {
	case KeyEnter:
		if s.picker.pick() {
			s.editor.Clear()
			s.beginModelFetch()
			s.dirty = true
		}
	case KeyEsc, KeyCtrlC, KeyCtrlP:
		s.cancelPicker()
	default:
		s.editor.Handle(k)
		s.picker.setFilter(s.editor.Text())
		s.dirty = true
	}
}

func (s *Session) modelKey(k Key) {
	if s.picker.menu.step(k) {
		s.dirty = true
		return
	}
	switch k.Type {
	case KeyEnter:
		s.submitModel()
	case KeyEsc, KeyCtrlC, KeyCtrlP:
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
	if s.pending != "" {
		s.editor.SetText(s.pending)
		s.pending = ""
	}
	s.clearDrawn()
}

func (s *Session) pickModelFirst(text string) {
	s.note((NoModelError{}).Error())
	if !s.startPicker() {
		if text != "" {
			s.editor.SetText(text)
		}
		s.dirty = true
		return
	}
	if text != "" {
		s.pending = text
		s.editor.Clear()
	}
	s.dirty = true
}

func (s *Session) turnKey(ctx context.Context, k Key) {
	switch k.Type {
	case KeyEsc:
		s.turnEsc()
	case KeyCtrlC:
		s.turnCtrlC()
	case KeyEnter:
		s.busyEnter(ctx)
	case KeyCtrlD, KeyCtrlL:
		if s.stopping && strings.TrimSpace(s.editor.Text()) != "" {
			s.submit(ctx)
		}
	case KeyUp:
		if s.editor.Text() == "" && len(s.queue) > 0 {
			s.takeBack()
			return
		}
		s.editor.Handle(k)
		s.refreshBusyLine()
	default:
		s.editor.Handle(k)
		s.refreshBusyLine()
	}
}

func (s *Session) turnEsc() {
	if s.stopping || s.escArmed {
		s.escArmed = false
		s.interrupt()
		s.refreshBusyLine()
		return
	}
	s.escArmed = true
	s.refreshBusyLine()
}

func (s *Session) turnCtrlC() {
	if s.editor.Text() != "" && !s.stopping {
		s.editor.Discard()
		s.refreshBusyLine()
		return
	}
	s.interrupt()
}

func (s *Session) secretKey(k Key) {
	switch k.Type {
	case KeyEnter:
		s.saveKey()
	case KeyEsc, KeyCtrlC:
		s.cancelKey()
	case KeyUp, KeyDown, KeyCtrlJ, KeyNewline:
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
	if s.cfg.Agent.Model() == "" {
		s.startPicker()
	}
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
	if k.Type == KeyShiftTab {
		s.cyclePermissions()
		return
	}
	if s.completionKey(k) {
		return
	}
	switch k.Type {
	case KeyCtrlP:
		s.openPickerKeepingDraft()
		return
	case KeyCtrlT:
		s.cycleEffort()
		return
	case KeyEsc:
		s.promptEsc()
		return
	}
	action := s.editor.Handle(k)
	switch action {
	case ActionSubmit:
		s.submit(ctx)
	case ActionInterrupt:
		s.promptInterrupt()
	case ActionEOF:
		s.quit = true
	case ActionRedraw:
		_ = s.renderer.Wipe()
		s.dirty = true
	case ActionCopy:
		s.copyKilled()
	default:
		s.dirty = true
	}
	s.updateCompletion()
}

func (s *Session) promptInterrupt() {
	s.dirty = true
	if s.editor.Text() != "" {
		s.editor.Discard()
		s.clearPromptCompletion()
		s.armed = false
		return
	}
	if s.armed {
		s.quit = true
		return
	}
	s.armed = true
}

func (s *Session) promptEsc() {
	if s.editor.Text() == "" {
		return
	}
	if !s.escArmed {
		s.escArmed = true
		return
	}
	s.escArmed = false
	s.editor.Discard()
	s.clearPromptCompletion()
	s.dirty = true
}

func (s *Session) openPickerKeepingDraft() {
	text := s.editor.Text()
	s.clearPromptCompletion()
	s.endPrompt()
	if !s.startPicker() {
		s.dirty = true
		return
	}
	s.pending = text
	s.editor.Clear()
	s.dirty = true
}

func (s *Session) submit(ctx context.Context) {
	if strings.TrimSpace(s.editor.Text()) == "" {
		s.editor.Clear()
		s.clearPromptCompletion()
		s.dirty = true
		return
	}
	s.dirty = true
	s.redraw()
	text := s.editor.Take()
	s.clearPromptCompletion()
	s.endPrompt()
	if name, arg, ok := splitSlash(text); ok {
		s.runSlash(ctx, name, arg)
		s.dirty = true
		return
	}
	if s.cfg.Agent.Model() == "" {
		s.pickModelFirst(text)
		return
	}
	if s.busy() {
		s.enqueue(promptText(text))
		return
	}
	s.startPrompt(ctx, promptText(text))
}

func (s *Session) startPrompt(ctx context.Context, prompt string) {
	s.startTurn(ctx, func(ctx context.Context, emit func(Event)) (Outcome, error) {
		return s.cfg.Agent.Send(ctx, prompt, emit)
	})
}

func (s *Session) busyEnter(ctx context.Context) {
	text := strings.TrimSpace(s.editor.Text())
	if text == "" {
		return
	}
	if _, _, ok := splitSlash(text); ok && !s.stopping {
		s.note(busySlashHint)
		return
	}
	s.submit(ctx)
	s.refreshBusyLine()
}

func (s *Session) enqueue(text string) {
	s.queue = append(s.queue, text)
	s.note(fmt.Sprintf(queuedNote, len(s.queue), cutRunes(firstLine(text), queuedNoteRunes)))
	s.refreshBusyLine()
}

func (s *Session) takeBack() {
	last := len(s.queue) - 1
	text := s.queue[last]
	s.editor.SetText(text)
	s.queue = s.queue[:last]
	s.note(fmt.Sprintf(tookBackNote, cutRunes(firstLine(text), queuedNoteRunes)))
	s.refreshBusyLine()
}

func (s *Session) refreshBusyLine() {
	var parts []string
	if n := len(s.queue); n > 0 {
		parts = append(parts, fmt.Sprintf("%d queued", n))
		if s.editor.Text() == "" {
			parts = append(parts, "↑ takes the last one back")
		}
	}
	if s.escArmed {
		parts = append(parts, escStopHint)
	}
	if typed := firstLine(s.editor.Text()); typed != "" {
		parts = append(parts, promptGlyph+tailRunes(typed, s.cfg.Width()/2))
	}
	suffix := ""
	if len(parts) > 0 {
		suffix = suffixJoin + strings.Join(parts, suffixJoin)
	}
	s.out.setSuffix(suffix)
	s.out.restoreActivity()
}

func (s *Session) copyKilled() {
	text := s.editor.Killed()
	if text == "" || s.masking {
		return
	}
	_, _ = io.WriteString(s.cfg.Out, clipboardWrite(text))
	s.copied = true
	s.dirty = true
}

func clipboardWrite(text string) string {
	return "\x1b]52;c;" + base64.StdEncoding.EncodeToString([]byte(text)) + "\x07"
}
