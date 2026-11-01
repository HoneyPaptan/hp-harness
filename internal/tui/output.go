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
)

type output struct {
	w       io.Writer
	color   bool
	midLine bool
	active  string
}

func (o *output) write(s string) {
	_, _ = io.WriteString(o.w, s)
}

func (o *output) paint(sgr, s string) string {
	if !o.color {
		return s
	}
	return sgr + s + sgrReset
}

func (o *output) activity() {
	o.showActivity(activityText)
}

func (o *output) showActivity(label string) {
	if o.active == label || o.midLine {
		return
	}
	o.clearActivity()
	o.write(o.paint(sgrDim, label))
	o.active = label
}

func (o *output) clearActivity() {
	if o.active == "" {
		return
	}
	o.write(eraseLine)
	o.active = ""
}

func (o *output) endLine() {
	o.clearActivity()
	if !o.midLine {
		return
	}
	o.write(rowSeparator)
	o.midLine = false
}

func (o *output) text(s string) {
	s = plainText(s)
	if s == "" {
		return
	}
	o.clearActivity()
	o.write(strings.ReplaceAll(s, "\n", rowSeparator))
	o.midLine = !strings.HasSuffix(s, "\n")
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
	line := fmt.Sprintf("%s  steps %d  tokens %d/%d  ttft %dms",
		o.Model, o.Steps, o.InputTokens, o.OutputTokens, o.TTFT.Milliseconds())
	for _, extra := range []string{o.Jev, o.Note} {
		if extra != "" {
			line += "  " + extra
		}
	}
	return line
}

func shortError(err error) string {
	line, _, _ := strings.Cut(err.Error(), "\n")
	runes := []rune(line)
	if len(runes) > maxErrorRunes {
		line = string(runes[:maxErrorRunes]) + "…"
	}
	return "error: " + line
}
