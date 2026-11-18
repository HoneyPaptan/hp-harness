package tui

import (
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

const (
	activityText = "working…"
	thinkingText = "thinking…"
	eraseLine    = "\r\x1b[K"
	sgrDim       = "\x1b[2m"
	sgrRed       = "\x1b[31m"
	sgrReset     = "\x1b[0m"
	reserveRow   = "\x1bD\x1b[A"
	saveCursor   = "\x1b7"
	loadCursor   = "\x1b8"
	belowRow     = "\r\n"
	eraseRest    = "\x1b[K"
	suffixJoin   = " · "
)

type output struct {
	w       io.Writer
	color   bool
	midLine bool
	active  string
	base    string
	suffix  string
	below   bool
	width   func() int
}

func (o *output) write(s string) {
	_, _ = io.WriteString(o.w, s)
}

func (o *output) paint(sgr, s string) string {
	return paint(o.color, sgr, s)
}

func paint(color bool, sgr, s string) string {
	if !color {
		return s
	}
	return sgr + s + sgrReset
}

func (o *output) activity() {
	o.showActivity(activityText)
}

func (o *output) showActivity(label string) {
	full := label + o.suffix
	if o.active == full || o.midLine {
		return
	}
	o.clearActivity()
	o.write(o.paint(sgrDim, full))
	o.active = full
	o.base = label
}

func (o *output) setSuffix(suffix string) {
	o.suffix = suffix
	if o.midLine {
		o.clearBelow()
		o.showBelow()
		return
	}
	if o.active == "" {
		return
	}
	base := o.base
	o.clearActivity()
	o.showActivity(base)
}

func (o *output) restoreActivity() {
	if o.base == "" {
		o.base = activityText
	}
	o.showActivity(o.base)
}

func (o *output) clearActivity() {
	if o.active == "" {
		return
	}
	o.write(eraseLine)
	o.active = ""
}

func (o *output) showBelow() {
	if !o.midLine || o.suffix == "" {
		return
	}
	text := strings.TrimPrefix(o.suffix, suffixJoin)
	if o.width != nil {
		text = fitRow(text, max(o.width()-1, 1))
	}
	o.write(reserveRow + saveCursor + belowRow + o.paint(sgrDim, text) + eraseRest + loadCursor)
	o.below = true
}

func (o *output) clearBelow() {
	if !o.below {
		return
	}
	o.write(saveCursor + belowRow + eraseRest + loadCursor)
	o.below = false
}

func (o *output) endLine() {
	o.clearActivity()
	if !o.midLine {
		return
	}
	o.clearBelow()
	o.write(rowSeparator)
	o.midLine = false
}

func (o *output) text(s string) {
	s = plainText(s)
	if s == "" {
		return
	}
	o.clearActivity()
	o.clearBelow()
	o.write(strings.ReplaceAll(s, "\n", rowSeparator))
	o.midLine = !strings.HasSuffix(s, "\n")
	o.showBelow()
}

func (o *output) line(sgr, s string) {
	o.endLine()
	o.write(o.paint(sgr, strings.ReplaceAll(plainText(s), "\n", rowSeparator)) + rowSeparator)
}

func (o *output) gap() {
	o.endLine()
	o.write(rowSeparator)
}

func (o *output) apply(ev Event) {
	switch ev.Kind {
	case EventText:
		o.text(ev.Text)
		return
	case EventThinking:
		o.showActivity(thinkingText)
		return
	case EventTool:
		o.line(sgrDim, "  "+ev.Text)
	case EventToolError:
		o.line(sgrRed, "  "+ev.Text)
	case EventNote:
		o.line(sgrDim, ev.Text)
	}
	o.activity()
}

func plainText(s string) string {
	var out strings.Builder
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if !isControl(r) {
			out.WriteString(s[i : i+size])
		}
		i += size
	}
	return out.String()
}

func isControl(r rune) bool {
	if r == '\n' || r == '\t' {
		return false
	}
	return r < 0x20 || (r >= 0x7f && r <= 0x9f)
}

func statusLine(o Outcome) string {
	line := fmt.Sprintf("%d in · %d out · ttft %d ms · %d steps",
		o.InputTokens, o.OutputTokens, o.TTFT.Milliseconds(), o.Steps)
	for _, extra := range []string{o.Jev, o.Note} {
		if extra != "" {
			line += " · " + extra
		}
	}
	return line
}

func shortError(err error) string {
	return "error: " + errorText(err)
}

func errorText(err error) string {
	text, _, _ := strings.Cut(err.Error(), "\n")
	runes := []rune(text)
	if len(runes) <= maxErrorRunes {
		return text
	}
	cut := string(runes[:maxErrorRunes])
	if i := strings.LastIndex(cut, " "); i > 0 {
		cut = cut[:i]
	}
	return cut + "…"
}
