package tui

import "strings"

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return line
}

func cutRunes(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max]) + "…"
}

func tailRunes(s string, max int) string {
	runes := []rune(s)
	if max < 1 || len(runes) <= max {
		return s
	}
	return "…" + string(runes[len(runes)-max:])
}
