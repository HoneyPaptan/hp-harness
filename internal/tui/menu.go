package tui

import "strings"

const menuRows = 8

type menu struct {
	items    []string
	filter   string
	selected int
	offset   int
}

func menuStep(k Key) int {
	switch k.Type {
	case KeyUp, KeyCtrlK:
		return -1
	case KeyDown, KeyCtrlJ:
		return 1
	}
	return 0
}

func (m *menu) step(k Key) bool {
	delta := menuStep(k)
	if delta == 0 {
		return false
	}
	m.move(delta)
	return true
}

func newMenu(items []string) *menu {
	return &menu{items: items}
}

func (m *menu) setFilter(text string) {
	m.filter = text
	m.selected = 0
	m.offset = 0
}

func (m *menu) matches() []string {
	if m.filter == "" {
		return m.items
	}
	low := strings.ToLower(m.filter)
	out := make([]string, 0, len(m.items))
	for _, item := range m.items {
		if strings.Contains(strings.ToLower(item), low) {
			out = append(out, item)
		}
	}
	return out
}

func (m *menu) move(delta int) {
	list := m.matches()
	n := len(list)
	if n == 0 {
		return
	}
	m.selected = (m.selected + delta + n) % n
	m.keepVisible()
}

func (m *menu) keepVisible() {
	list := m.matches()
	n := len(list)
	if m.selected < m.offset {
		m.offset = m.selected
	}
	if m.selected >= m.offset+menuRows {
		m.offset = m.selected - menuRows + 1
	}
	if n > menuRows && m.offset > n-menuRows {
		m.offset = n - menuRows
	}
	if m.offset < 0 {
		m.offset = 0
	}
}

func (m *menu) window() (rows []string, selected int) {
	list := m.matches()
	if len(list) == 0 {
		return nil, -1
	}
	end := m.offset + menuRows
	if end > len(list) {
		end = len(list)
	}
	rows = list[m.offset:end]
	return rows, m.selected - m.offset
}

func (m *menu) moreHidden() int {
	list := m.matches()
	if len(list) == 0 {
		return 0
	}
	end := m.offset + menuRows
	if end > len(list) {
		end = len(list)
	}
	return len(list) - (end - m.offset)
}

func (m *menu) chosen() (string, bool) {
	list := m.matches()
	if len(list) == 0 {
		return "", false
	}
	return list[m.selected], true
}
