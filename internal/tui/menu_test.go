package tui

import "testing"

func TestMenuFilter(t *testing.T) {
	m := newMenu([]string{"OpenAI", "groq", "XAI"})
	m.setFilter("o")
	got := m.matches()
	want := []string{"OpenAI", "groq"}
	if len(got) != len(want) {
		t.Fatalf("matches %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("matches[%d] %q want %q", i, got[i], want[i])
		}
	}
	m.setFilter("GRO")
	if got := m.matches(); len(got) != 1 || got[0] != "groq" {
		t.Fatalf("case insensitive %v", got)
	}
}

func TestMenuFilterResetsSelection(t *testing.T) {
	m := newMenu([]string{"a", "b", "c"})
	m.move(2)
	m.setFilter("b")
	if m.selected != 0 {
		t.Fatalf("selected %d want 0", m.selected)
	}
	if m.offset != 0 {
		t.Fatalf("offset %d want 0", m.offset)
	}
}

func TestMenuFilterNoMatch(t *testing.T) {
	m := newMenu([]string{"a", "b"})
	m.setFilter("zzz")
	if _, ok := m.chosen(); ok {
		t.Fatal("chosen with no match")
	}
	rows, sel := m.window()
	if len(rows) != 0 || sel != -1 {
		t.Fatalf("window %v sel %d", rows, sel)
	}
}

func TestMenuMoveWraps(t *testing.T) {
	tests := []struct {
		name  string
		items []string
		moves []int
		want  int
	}{
		{"down", []string{"a", "b", "c"}, []int{1}, 1},
		{"down past end", []string{"a", "b", "c"}, []int{1, 1, 1}, 0},
		{"up wraps", []string{"a", "b", "c"}, []int{-1}, 2},
		{"empty", nil, []int{1, -1}, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := newMenu(tc.items)
			for _, d := range tc.moves {
				m.move(d)
			}
			if m.selected != tc.want {
				t.Fatalf("selected %d want %d", m.selected, tc.want)
			}
		})
	}
}

func TestMenuMoveEmpty(t *testing.T) {
	m := newMenu(nil)
	m.move(1)
	m.move(-1)
}

func TestMenuWindowScroll(t *testing.T) {
	items := make([]string, 12)
	for i := range items {
		items[i] = string(rune('a' + i))
	}
	m := newMenu(items)
	for range 8 {
		m.move(1)
	}
	rows, sel := m.window()
	if len(rows) != menuRows {
		t.Fatalf("rows %d want %d", len(rows), menuRows)
	}
	if sel != 7 {
		t.Fatalf("relative selected %d want 7", sel)
	}
	if m.moreHidden() != 4 {
		t.Fatalf("moreHidden %d want 4", m.moreHidden())
	}
	for range 8 {
		m.move(-1)
	}
	rows, sel = m.window()
	if m.offset != 0 {
		t.Fatalf("offset %d want 0", m.offset)
	}
	if sel != 0 {
		t.Fatalf("relative selected %d want 0", sel)
	}
	if m.moreHidden() != 4 {
		t.Fatalf("moreHidden %d want 4", m.moreHidden())
	}
}

func TestMenuWindowSelectedRelative(t *testing.T) {
	m := newMenu([]string{"a", "b", "c"})
	m.move(1)
	_, sel := m.window()
	if sel != 1 {
		t.Fatalf("sel %d want 1", sel)
	}
}

func TestMenuMoreHiddenFits(t *testing.T) {
	m := newMenu([]string{"a", "b", "c"})
	if m.moreHidden() != 0 {
		t.Fatalf("moreHidden %d want 0", m.moreHidden())
	}
}

func TestMenuChosen(t *testing.T) {
	m := newMenu([]string{"groq", "ollama"})
	m.move(1)
	got, ok := m.chosen()
	if !ok || got != "ollama" {
		t.Fatalf("chosen %q ok %v", got, ok)
	}
}
