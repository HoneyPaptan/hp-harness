package tui

import "testing"

func TestPickerMoveWraps(t *testing.T) {
	tests := []struct {
		name  string
		items []string
		moves []int
		want  int
	}{
		{"down", []string{"a", "b", "c"}, []int{1}, 1},
		{"down past the end wraps", []string{"a", "b", "c"}, []int{1, 1, 1}, 0},
		{"up wraps", []string{"a", "b", "c"}, []int{-1}, 2},
		{"up twice wraps", []string{"a", "b", "c"}, []int{-1, -1}, 1},
		{"empty stays put", nil, []int{1, -1}, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := newPicker(tc.items)
			for _, delta := range tc.moves {
				p.move(delta)
			}
			if p.selected != tc.want {
				t.Fatalf("selected %d want %d", p.selected, tc.want)
			}
		})
	}
}

func TestPickerChosen(t *testing.T) {
	if got := newPicker(nil).chosen(); got != "" {
		t.Fatalf("empty picker chose %q", got)
	}
	p := newPicker([]string{"groq", "ollama"})
	p.move(1)
	if got := p.chosen(); got != "ollama" {
		t.Fatalf("chose %q", got)
	}
}

func TestPickerPickMovesToTheModelStage(t *testing.T) {
	p := newPicker([]string{"groq", "ollama"})
	p.move(1)
	p.pick()
	if p.stage != pickModel || p.provider != "ollama" {
		t.Fatalf("stage %v provider %q", p.stage, p.provider)
	}
}
