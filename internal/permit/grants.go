package permit

type Grants struct {
	items []Action
}

func (g *Grants) Add(a Action) {
	if g == nil {
		return
	}
	g.items = append(g.items, a)
}

func (g *Grants) Has(a Action) bool {
	if g == nil {
		return false
	}
	for _, item := range g.items {
		if item.Tool == a.Tool && item.Target == a.Target {
			return true
		}
	}
	return false
}
