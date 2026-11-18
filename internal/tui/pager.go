package tui

import (
	"fmt"
	"strings"
)

const (
	pagerRows   = 16
	pagerHint   = "↑ ↓ PgUp PgDn scroll · g top · G bottom · Esc back"
	detailTitle = "full detail"
	detailEmpty = "nothing to show yet: send a prompt first"
	detailBusy  = "full detail opens when the turn is over"
)

type pager struct {
	title string
	lines []string
	top   int
}

func newPager(title string, lines []string, width int) *pager {
	var wrapped []string
	for _, line := range lines {
		line = strings.ReplaceAll(strings.TrimRight(line, "\r"), "\t", tabCells)
		wrapped = append(wrapped, wrapCells(plainText(line), width-len(pickerPad))...)
	}
	return &pager{title: title, lines: wrapped}
}

func wrapCells(line string, width int) []string {
	if width < 1 || rowWidth(line) <= width {
		return []string{line}
	}
	var rows []string
	var row strings.Builder
	col := 0
	for _, c := range line {
		cells := RuneWidth(c)
		if col > 0 && col+cells > width {
			rows = append(rows, row.String())
			row.Reset()
			col = 0
		}
		row.WriteRune(c)
		col += cells
	}
	return append(rows, row.String())
}

func (p *pager) scroll(to int) {
	p.top = max(0, min(to, len(p.lines)-pagerRows))
}

func (p *pager) key(k Key) bool {
	switch {
	case k.Type == KeyUp || k.Type == KeyRune && k.Rune == 'k':
		p.scroll(p.top - 1)
	case k.Type == KeyDown || k.Type == KeyRune && k.Rune == 'j':
		p.scroll(p.top + 1)
	case k.Type == KeyPgUp || k.Type == KeyRune && k.Rune == 'b':
		p.scroll(p.top - pagerRows)
	case k.Type == KeyPgDn || k.Type == KeyRune && k.Rune == ' ':
		p.scroll(p.top + pagerRows)
	case k.Type == KeyHome || k.Type == KeyRune && k.Rune == 'g':
		p.scroll(0)
	case k.Type == KeyEnd || k.Type == KeyRune && k.Rune == 'G':
		p.scroll(len(p.lines))
	case k.Type == KeyEsc || k.Type == KeyCtrlC || k.Type == KeyCtrlO || k.Type == KeyRune && k.Rune == 'q':
		return true
	}
	return false
}

func (s *Session) drawPager(p *pager) {
	end := min(p.top+pagerRows, len(p.lines))
	prompt := fmt.Sprintf("%s · lines %d-%d of %d", p.title, min(p.top+1, end), end, len(p.lines))
	_ = s.renderer.DrawMenu(prompt, nil, 0, p.lines[p.top:end], -1, "", pagerHint, s.footer())
}

func (s *Session) openDiff(title string, lines []string) {
	s.view.diff = newPager("diff "+title, lines, s.cfg.Width())
	s.dirty = true
}

func (s *Session) diffKey(k Key) {
	if s.view.diff.key(k) {
		s.view.diff = nil
	}
	s.dirty = true
}

func (s *Session) openDetail() {
	lines, err := s.cfg.Agent.Detail()
	if err != nil {
		s.clearDrawn()
		s.fail(err)
		s.dirty = true
		return
	}
	if len(lines) == 0 {
		s.clearDrawn()
		s.note(detailEmpty)
		s.dirty = true
		return
	}
	s.clearPromptCompletion()
	s.detail = newPager(detailTitle, lines, s.cfg.Width())
	s.detail.scroll(len(s.detail.lines))
	s.dirty = true
}

func (s *Session) detailKey(k Key) {
	if s.detail.key(k) {
		s.detail = nil
	}
	s.dirty = true
}
