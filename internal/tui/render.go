package tui

import (
	"fmt"
	"io"
	"slices"
	"strings"
)

const (
	defaultWidth = 80
	eraseBelow   = "\x1b[J"
	pasteOn      = "\x1b[?2004h"
	pasteOff     = "\x1b[?2004l"
	rowSeparator = "\r\n"
	tabCells     = "    "
)

func EnableBracketedPaste(w io.Writer) error {
	_, err := io.WriteString(w, pasteOn)
	return err
}

func DisableBracketedPaste(w io.Writer) error {
	_, err := io.WriteString(w, pasteOff)
	return err
}

type Renderer struct {
	w         io.Writer
	width     int
	cursorRow int
	lastRow   int
}

func NewRenderer(w io.Writer, width int) *Renderer {
	r := &Renderer{w: w}
	r.SetWidth(width)
	return r
}

func (r *Renderer) SetWidth(width int) {
	if width <= 0 {
		width = defaultWidth
	}
	r.width = width
}

func (r *Renderer) Draw(prompt string, buf []rune, cursor int) error {
	head := []rune(prompt)
	view := layOut(slices.Concat(head, buf), len(head)+cursor, r.width)
	var out strings.Builder
	moveUp(&out, r.cursorRow)
	out.WriteString("\r" + eraseBelow)
	out.WriteString(strings.Join(view.rows, rowSeparator))
	moveUp(&out, len(view.rows)-1-view.row)
	out.WriteString("\r")
	if view.col > 0 {
		fmt.Fprintf(&out, "\x1b[%dC", view.col)
	}
	r.cursorRow = view.row
	r.lastRow = len(view.rows) - 1
	_, err := io.WriteString(r.w, out.String())
	return err
}

func (r *Renderer) Finish() error {
	var out strings.Builder
	moveDown(&out, r.lastRow-r.cursorRow)
	out.WriteString(rowSeparator)
	r.cursorRow = 0
	r.lastRow = 0
	_, err := io.WriteString(r.w, out.String())
	return err
}

func (r *Renderer) Clear() error {
	var out strings.Builder
	moveUp(&out, r.cursorRow)
	out.WriteString("\r" + eraseBelow)
	r.cursorRow = 0
	r.lastRow = 0
	_, err := io.WriteString(r.w, out.String())
	return err
}

func moveUp(out *strings.Builder, rows int) {
	if rows > 0 {
		fmt.Fprintf(out, "\x1b[%dA", rows)
	}
}

func moveDown(out *strings.Builder, rows int) {
	if rows > 0 {
		fmt.Fprintf(out, "\x1b[%dB", rows)
	}
}

type layout struct {
	rows     []string
	row, col int
}

func layOut(text []rune, cursor, width int) layout {
	var view layout
	var row strings.Builder
	col := 0
	endRow := func() {
		view.rows = append(view.rows, row.String())
		row.Reset()
		col = 0
	}
	for i, c := range text {
		cell, cells := glyph(c)
		if c != '\n' && col > 0 && col+cells > width {
			endRow()
		}
		if i == cursor {
			view.row, view.col = len(view.rows), col
		}
		if c == '\n' {
			endRow()
			continue
		}
		row.WriteString(cell)
		col += cells
	}
	if cursor >= len(text) {
		if col >= width {
			endRow()
		}
		view.row, view.col = len(view.rows), col
	}
	endRow()
	return view
}

func glyph(r rune) (string, int) {
	switch {
	case r == '\n':
		return "", 0
	case r == '\t':
		return tabCells, len(tabCells)
	case r == 0x7f:
		return "^?", 2
	case r < 0x20:
		return "^" + string(r+0x40), 2
	case r >= 0x80 && r <= 0x9f:
		return "\ufffd", 1
	}
	return string(r), RuneWidth(r)
}
