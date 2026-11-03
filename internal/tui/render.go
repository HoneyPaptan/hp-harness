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
	pickerMark   = "› "
	pickerPad    = "  "
	maskRune     = '•'
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
	color     bool
	cursorRow int
	promptRow int
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

func (r *Renderer) SetColor(color bool) {
	r.color = color
}

func (r *Renderer) Draw(prompt string, buf []rune, cursor int, footer string) error {
	return r.draw(prompt, buf, cursor, footer, false)
}

func (r *Renderer) DrawMasked(prompt string, buf []rune, cursor int, footer string) error {
	return r.draw(prompt, buf, cursor, footer, true)
}

func (r *Renderer) draw(prompt string, buf []rune, cursor int, footer string, mask bool) error {
	if mask {
		buf = maskedRunes(buf)
	}
	head := []rune(prompt)
	view := layOut(slices.Concat(head, buf), len(head)+cursor, r.width)
	frame := view.rows
	if footer != "" {
		frame = append(slices.Clone(view.rows), "", r.footerRow(footer))
	}
	var out strings.Builder
	moveUp(&out, r.cursorRow)
	out.WriteString("\r" + eraseBelow)
	out.WriteString(strings.Join(frame, rowSeparator))
	moveUp(&out, len(frame)-1-view.row)
	out.WriteString("\r")
	if view.col > 0 {
		fmt.Fprintf(&out, "\x1b[%dC", view.col)
	}
	r.cursorRow, r.promptRow, r.lastRow = view.row, len(view.rows)-1, len(frame)-1
	_, err := io.WriteString(r.w, out.String())
	return err
}

func maskedRunes(buf []rune) []rune {
	out := make([]rune, len(buf))
	for i := range out {
		out[i] = maskRune
	}
	return out
}

func (r *Renderer) DrawList(title string, items []string, selected int, footer string) error {
	frame := make([]string, 0, len(items)+3)
	frame = append(frame, r.dimRow(title))
	for i, item := range items {
		frame = append(frame, r.listRow(i == selected, item))
	}
	frame = append(frame, "")
	if footer != "" {
		frame = append(frame, r.footerRow(footer))
	}
	var out strings.Builder
	moveUp(&out, r.cursorRow)
	out.WriteString("\r" + eraseBelow)
	out.WriteString(strings.Join(frame, rowSeparator))
	r.cursorRow, r.promptRow, r.lastRow = len(frame)-1, len(frame)-1, len(frame)-1
	_, err := io.WriteString(r.w, out.String())
	return err
}

func (r *Renderer) dimRow(text string) string {
	return paint(r.color, sgrDim, fitRow(text, r.width))
}

func (r *Renderer) listRow(selected bool, text string) string {
	mark := pickerPad
	if selected {
		mark = pickerMark
	}
	return fitRow(mark+text, r.width)
}

func (r *Renderer) footerRow(text string) string {
	return paint(r.color, sgrDim, fitRow(text, r.width))
}

func (r *Renderer) Finish() error {
	var out strings.Builder
	moveDown(&out, r.promptRow-r.cursorRow)
	out.WriteString(rowSeparator)
	if r.lastRow > r.promptRow {
		out.WriteString(eraseBelow)
	}
	r.cursorRow, r.promptRow, r.lastRow = 0, 0, 0
	_, err := io.WriteString(r.w, out.String())
	return err
}

func (r *Renderer) Clear() error {
	var out strings.Builder
	moveUp(&out, r.cursorRow)
	out.WriteString("\r" + eraseBelow)
	r.cursorRow, r.promptRow, r.lastRow = 0, 0, 0
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

func fitRow(text string, width int) string {
	if rowWidth(text) <= width {
		return text
	}
	var out strings.Builder
	col := 0
	for _, c := range text {
		cells := RuneWidth(c)
		if col+cells > width-1 {
			break
		}
		out.WriteRune(c)
		col += cells
	}
	out.WriteString("…")
	return out.String()
}

func rowWidth(text string) int {
	col := 0
	for _, c := range text {
		col += RuneWidth(c)
	}
	return col
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
