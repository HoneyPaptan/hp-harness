package tui

const (
	pickerTitle = "  pick a provider (↑ ↓ Enter, Esc cancels)"
	pickerNote  = "model unchanged"
	pickerEmpty = "no provider has a key, run hp key set NAME"
)

type pickStage int

const (
	pickProvider pickStage = iota
	pickModel
)

type picker struct {
	items    []string
	selected int
	stage    pickStage
	provider string
}

func newPicker(items []string) *picker {
	return &picker{items: items}
}

func (p *picker) move(delta int) {
	if len(p.items) == 0 {
		return
	}
	p.selected = (p.selected + delta + len(p.items)) % len(p.items)
}

func (p *picker) chosen() string {
	if len(p.items) == 0 {
		return ""
	}
	return p.items[p.selected]
}

func (p *picker) pick() {
	p.provider = p.chosen()
	p.stage = pickModel
}
