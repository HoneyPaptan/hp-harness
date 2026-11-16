package tui

import (
	"context"
	"strings"
)

const (
	pickerLoading  = "loading models"
	modelsNoneNote = "models: none listed"
	modelsNotePref = "models: "
)

const (
	freeSuffix = "-free"
	freeTag    = "  free"
)

type modelsResult struct {
	picker *picker
	ids    []string
	err    error
}

func modelPrompt(provider string) string {
	return "model for " + provider + ": "
}

func providerOnlyArg(arg string) string {
	arg = strings.TrimSpace(arg)
	if arg == "" {
		return ""
	}
	if !strings.Contains(arg, "/") {
		return arg
	}
	if strings.HasSuffix(arg, "/") {
		name := strings.TrimSuffix(arg, "/")
		if !strings.Contains(name, "/") {
			return name
		}
	}
	return ""
}

func modelsFetchNote(err error) string {
	return modelsNotePref + errorText(err)
}

func markFree(rows []string) []string {
	out := make([]string, len(rows))
	for i, row := range rows {
		out[i] = row
		if strings.HasSuffix(row, freeSuffix) {
			out[i] += freeTag
		}
	}
	return out
}

func modelPickerMore(p *picker) string {
	if p.loading {
		return pickerLoading
	}
	return pickerMore(p.menu)
}

func (s *Session) beginModelFetch() {
	if s.picker == nil {
		return
	}
	if s.picker.loadCancel != nil {
		s.picker.loadCancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.picker.loadCancel = cancel
	s.picker.loading = true
	p := s.picker
	go func() {
		ids, err := s.cfg.Agent.Models(ctx, p.provider)
		res := modelsResult{picker: p, ids: ids, err: err}
		select {
		case s.models <- res:
		case <-s.stopped:
		}
	}()
}

func (s *Session) handleModelsResult(res modelsResult) {
	if s.picker == nil || res.picker != s.picker {
		return
	}
	s.picker.loading = false
	if res.err != nil {
		s.picker.modelsNote = modelsFetchNote(res.err)
		s.dirty = true
		return
	}
	if len(res.ids) == 0 {
		s.picker.modelsNote = modelsNoneNote
		s.dirty = true
		return
	}
	s.picker.modelsNote = ""
	s.picker.menu = newMenu(res.ids)
	s.picker.setFilter(s.editor.Text())
	s.dirty = true
}

func (s *Session) startPickerAtProvider(name string) {
	s.picker = &picker{
		menu:     newMenu(nil),
		stage:    pickModel,
		provider: name,
		loading:  true,
	}
	s.editor.Clear()
	s.beginModelFetch()
	s.dirty = true
}

func (s *Session) cancelModelFetch() {
	if s.picker != nil && s.picker.loadCancel != nil {
		s.picker.loadCancel()
		s.picker.loadCancel = nil
	}
}
