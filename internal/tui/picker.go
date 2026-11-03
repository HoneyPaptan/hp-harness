package tui

import "strconv"

const (
	providerPrompt = "provider: "
	pickerHint     = "↑ ↓ Enter, Esc cancels"
	pickerNote     = "model unchanged"
	pickerEmpty    = "no provider has a key, run hp key set NAME"
)

type pickStage int

const (
	pickProvider pickStage = iota
	pickModel
)

type picker struct {
	menu     *menu
	stage    pickStage
	provider string
}

func newPicker(items []string) *picker {
	return &picker{menu: newMenu(items)}
}

func (p *picker) setFilter(text string) {
	p.menu.setFilter(text)
}

func (p *picker) move(delta int) {
	p.menu.move(delta)
}

func (p *picker) pick() bool {
	chosen, ok := p.menu.chosen()
	if !ok {
		return false
	}
	p.provider = chosen
	p.stage = pickModel
	return true
}

func pickerMore(m *menu) string {
	if n := m.moreHidden(); n > 0 {
		return "+" + strconv.Itoa(n) + " more · " + pickerHint
	}
	return pickerHint
}
