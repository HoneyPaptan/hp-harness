package crew

import (
	"path/filepath"
	"strings"
)

const maxLabelRunes = 40

func (s *State) label(getenv func(string) string) string {
	if text := shortLabel(s.Label); text != "" {
		return text
	}
	if filepath.Dir(s.Plan) == plansDir(getenv) && len(s.Tasks) > 0 {
		return shortLabel(s.Tasks[0].Title)
	}
	return filepath.Base(s.Plan)
}

func shortLabel(text string) string {
	text = strings.Join(strings.Fields(text), " ")
	runes := []rune(text)
	if len(runes) <= maxLabelRunes {
		return text
	}
	return strings.TrimSpace(string(runes[:maxLabelRunes-1])) + "…"
}
