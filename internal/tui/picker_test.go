package tui

import "testing"

func TestPickerPickMovesToTheModelStage(t *testing.T) {
	p := newPicker([]string{"groq", "ollama"})
	p.move(1)
	if !p.pick() {
		t.Fatal("pick failed")
	}
	if p.stage != pickModel || p.provider != "ollama" {
		t.Fatalf("stage %v provider %q", p.stage, p.provider)
	}
}

func TestPickerPickNoMatch(t *testing.T) {
	p := newPicker([]string{"groq"})
	p.setFilter("zzz")
	if p.pick() {
		t.Fatal("pick with no match")
	}
}

func TestPickerSetFilter(t *testing.T) {
	p := newPicker([]string{"openai", "groq", "xai"})
	p.setFilter("gr")
	got, ok := p.menu.chosen()
	if !ok || got != "groq" {
		t.Fatalf("chosen %q ok %v", got, ok)
	}
}
