package tui

import (
	"slices"
	"unicode"
)

type Action int

const (
	ActionNone Action = iota
	ActionSubmit
	ActionInterrupt
	ActionEOF
	ActionRedraw
)

type Editor struct {
	buf      []rune
	cursor   int
	history  []string
	position int
	draft    []rune
}

func (e *Editor) Text() string {
	return string(e.buf)
}

func (e *Editor) Runes() []rune {
	return e.buf
}

func (e *Editor) Cursor() int {
	return e.cursor
}

func (e *Editor) Clear() {
	e.buf = nil
	e.cursor = 0
	e.draft = nil
	e.position = len(e.history)
}

func (e *Editor) Take() string {
	text := e.Text()
	if text != "" && (len(e.history) == 0 || e.history[len(e.history)-1] != text) {
		e.history = append(e.history, text)
	}
	e.Clear()
	return text
}

func (e *Editor) Handle(k Key) Action {
	switch k.Type {
	case KeyEnter:
		return ActionSubmit
	case KeyCtrlC:
		return ActionInterrupt
	case KeyCtrlL:
		return ActionRedraw
	case KeyCtrlD:
		if len(e.buf) == 0 {
			return ActionEOF
		}
		e.remove(e.cursor, e.cursor+1)
	default:
		e.edit(k)
	}
	return ActionNone
}

func (e *Editor) edit(k Key) {
	switch k.Type {
	case KeyRune:
		e.insert(k.Rune)
	case KeyBackspace:
		e.remove(e.cursor-1, e.cursor)
	case KeyDelete:
		e.remove(e.cursor, e.cursor+1)
	case KeyLeft:
		e.cursor = max(e.cursor-1, 0)
	case KeyRight:
		e.cursor = min(e.cursor+1, len(e.buf))
	case KeyHome, KeyCtrlA:
		e.cursor = e.lineStart()
	case KeyEnd, KeyCtrlE:
		e.cursor = e.lineEnd()
	case KeyCtrlU:
		e.remove(e.lineStart(), e.cursor)
	case KeyCtrlK:
		e.remove(e.cursor, e.lineEnd())
	case KeyCtrlW:
		e.remove(e.wordStart(), e.cursor)
	case KeyUp:
		e.recall(e.position - 1)
	case KeyDown:
		e.recall(e.position + 1)
	}
}

func (e *Editor) insert(r rune) {
	e.buf = slices.Insert(e.buf, e.cursor, r)
	e.cursor++
}

func (e *Editor) remove(from, to int) {
	from = max(from, 0)
	to = min(to, len(e.buf))
	if from >= to {
		return
	}
	e.buf = slices.Delete(e.buf, from, to)
	e.cursor = from
}

func (e *Editor) lineStart() int {
	i := e.cursor
	for i > 0 && e.buf[i-1] != '\n' {
		i--
	}
	return i
}

func (e *Editor) lineEnd() int {
	i := e.cursor
	for i < len(e.buf) && e.buf[i] != '\n' {
		i++
	}
	return i
}

func (e *Editor) wordStart() int {
	i := e.cursor
	for i > 0 && unicode.IsSpace(e.buf[i-1]) {
		i--
	}
	for i > 0 && !unicode.IsSpace(e.buf[i-1]) {
		i--
	}
	return i
}

func (e *Editor) recall(position int) {
	if position < 0 || position > len(e.history) {
		return
	}
	if e.position == len(e.history) {
		e.draft = slices.Clone(e.buf)
	}
	e.position = position
	if position == len(e.history) {
		e.buf = slices.Clone(e.draft)
	} else {
		e.buf = []rune(e.history[position])
	}
	e.cursor = len(e.buf)
}
