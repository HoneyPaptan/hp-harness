package tool

import "github.com/HoneyPaptan/hp-harness/internal/provider"

type Registry struct {
	order []string
	tools map[string]Tool
}

func NewRegistry(tools ...Tool) *Registry {
	r := &Registry{tools: make(map[string]Tool, len(tools))}
	for _, t := range tools {
		if _, ok := r.tools[t.Name]; ok {
			continue
		}
		r.order = append(r.order, t.Name)
		r.tools[t.Name] = t
	}
	return r
}

func (r *Registry) Get(name string) (Tool, bool) {
	t, ok := r.tools[name]
	return t, ok
}

func (r *Registry) Defs() []provider.ToolDef {
	out := make([]provider.ToolDef, len(r.order))
	for i, name := range r.order {
		t := r.tools[name]
		out[i] = provider.ToolDef{Name: t.Name, Description: t.Description, Schema: t.Schema}
	}
	return out
}
