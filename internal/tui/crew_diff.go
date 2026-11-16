package tui

import (
	"fmt"
	"strings"
)

const (
	diffRows = 16
	diffHint = "↑ ↓ PgUp PgDn scroll · g top · G bottom · Esc back"
)

type diffView struct {
	title string
	lines []string
	top   int
}

func (s *Session) openDiff(title string, lines []string) {
	for i, line := range lines {
		lines[i] = strings.ReplaceAll(strings.TrimRight(line, "\r"), "\t", "    ")
	}
	s.view.diff = &diffView{title: title, lines: lines}
	s.dirty = true
}

func (d *diffView) scroll(to int) {
	d.top = max(0, min(to, len(d.lines)-diffRows))
}

func (s *Session) diffKey(k Key) {
	d := s.view.diff
	switch {
	case k.Type == KeyUp || k.Type == KeyRune && k.Rune == 'k':
		d.scroll(d.top - 1)
	case k.Type == KeyDown || k.Type == KeyRune && k.Rune == 'j':
		d.scroll(d.top + 1)
	case k.Type == KeyPgUp || k.Type == KeyRune && k.Rune == 'b':
		d.scroll(d.top - diffRows)
	case k.Type == KeyPgDn || k.Type == KeyRune && k.Rune == ' ':
		d.scroll(d.top + diffRows)
	case k.Type == KeyRune && k.Rune == 'g':
		d.scroll(0)
	case k.Type == KeyRune && k.Rune == 'G':
		d.scroll(len(d.lines))
	case k.Type == KeyEsc || k.Type == KeyCtrlC || k.Type == KeyRune && k.Rune == 'q':
		s.view.diff = nil
	}
	s.dirty = true
}

func (s *Session) drawDiff() {
	d := s.view.diff
	end := min(d.top+diffRows, len(d.lines))
	prompt := fmt.Sprintf("diff %s · lines %d-%d of %d", d.title, min(d.top+1, end), end, len(d.lines))
	_ = s.renderer.DrawMenu(prompt, nil, 0, d.lines[d.top:end], -1, "", diffHint, s.footer())
}
