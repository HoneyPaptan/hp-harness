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
	killed   []rune
	browsing bool
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

func (e *Editor) SetText(text string) {
	e.buf = []rune(text)
	e.cursor = len(e.buf)
}

func (e *Editor) Replace(start, end int, text string) {
	start = max(start, 0)
	end = min(end, len(e.buf))
	insert := []rune(text)
	e.buf = slices.Concat(slices.Clone(e.buf[:start]), insert, e.buf[end:])
	e.cursor = start + len(insert)
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
	browsing := e.browsing
	e.browsing = false
	switch k.Type {
	case KeyEnter:
		if e.continueLine() {
			return ActionNone
		}
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
	case KeyUp:
		e.vertical(-1, browsing)
	case KeyDown:
		e.vertical(1, browsing)
	case KeyCtrlN:
		e.recall(e.position + 1)
	default:
		e.edit(k)
	}
	return ActionNone
}

func (e *Editor) edit(k Key) {
	switch k.Type {
	case KeyRune:
		e.insert(k.Rune)
	case KeyNewline, KeyCtrlJ:
		e.insert('\n')
	case KeyBackspace:
		e.remove(e.cursor-1, e.cursor)
	case KeyDelete:
		e.remove(e.cursor, e.cursor+1)
	case KeyLeft, KeyCtrlB:
		e.cursor = max(e.cursor-1, 0)
	case KeyRight, KeyCtrlF:
		e.cursor = min(e.cursor+1, len(e.buf))
	case KeyWordLeft:
		e.cursor = e.wordLeft()
	case KeyWordRight:
		e.cursor = e.wordRight()
	case KeyHome, KeyCtrlA:
		e.cursor = e.lineStart()
	case KeyEnd, KeyCtrlE:
		e.cursor = e.lineEnd()
	case KeyDraftStart:
		e.cursor = 0
	case KeyDraftEnd:
		e.cursor = len(e.buf)
	case KeyCtrlU:
		e.killLineStart()
	case KeyCtrlK:
		e.kill(e.cursor, e.lineEnd())
	case KeyCtrlW:
		e.kill(e.wordStart(), e.cursor)
	case KeyWordBackspace:
		e.remove(e.wordLeft(), e.cursor)
	case KeyWordDelete:
		e.remove(e.cursor, e.wordRight())
	case KeyCtrlY:
		e.yank()
	}
}

func (e *Editor) continueLine() bool {
	if e.cursor == 0 || e.buf[e.cursor-1] != '\\' {
		return false
	}
	e.buf[e.cursor-1] = '\n'
	return true
}

func (e *Editor) vertical(delta int, browsing bool) {
	if browsing || len(e.buf) == 0 {
		e.recall(e.position + delta)
		return
	}
	if e.moveLine(delta) {
		return
	}
	edge := 0
	if delta > 0 {
		edge = len(e.buf)
	}
	if e.cursor != edge {
		e.cursor = edge
		return
	}
	e.recall(e.position + delta)
}

func (e *Editor) moveLine(delta int) bool {
	start, end := e.lineStart(), e.lineEnd()
	col := e.cursor - start
	if delta < 0 {
		if start == 0 {
			return false
		}
		e.cursor = start - 1
		prev := e.lineStart()
		e.cursor = prev + min(col, start-1-prev)
		return true
	}
	if end == len(e.buf) {
		return false
	}
	e.cursor = end + 1
	next := e.lineEnd()
	e.cursor = end + 1 + min(col, next-end-1)
	return true
}

func (e *Editor) kill(from, to int) {
	from = max(from, 0)
	to = min(to, len(e.buf))
	if from >= to {
		return
	}
	e.killed = slices.Clone(e.buf[from:to])
	e.remove(from, to)
}

func (e *Editor) killLineStart() {
	start := e.lineStart()
	if start == e.cursor {
		e.remove(e.cursor-1, e.cursor)
		return
	}
	e.kill(start, e.cursor)
}

func (e *Editor) yank() {
	e.buf = slices.Insert(e.buf, e.cursor, e.killed...)
	e.cursor += len(e.killed)
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

func isWordRune(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

func (e *Editor) wordLeft() int {
	i := e.cursor
	for i > 0 && !isWordRune(e.buf[i-1]) {
		i--
	}
	for i > 0 && isWordRune(e.buf[i-1]) {
		i--
	}
	return i
}

func (e *Editor) wordRight() int {
	i := e.cursor
	for i < len(e.buf) && !isWordRune(e.buf[i]) {
		i++
	}
	for i < len(e.buf) && isWordRune(e.buf[i]) {
		i++
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
	e.browsing = true
}
