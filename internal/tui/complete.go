package tui

import (
	"strconv"
	"strings"
	"unicode"
)

const (
	completeNone = iota
	completeSlash
	completeFile
)

type completion struct {
	menu  *menu
	kind  int
	start int
	end   int
}

func (c *completion) visible() bool {
	return c != nil && len(c.menu.matches()) > 0
}

func completionMore(m *menu) string {
	if n := m.moreHidden(); n > 0 {
		return "+" + strconv.Itoa(n) + " more"
	}
	return ""
}

func tokenAt(text []rune, cursor int) (kind, start, end int, word string, ok bool) {
	if cursor < 0 {
		cursor = 0
	}
	if cursor > len(text) {
		cursor = len(text)
	}
	if s, e, w, yes := slashToken(text, cursor); yes {
		return completeSlash, s, e, w, true
	}
	if s, e, w, yes := fileToken(text, cursor); yes {
		return completeFile, s, e, w, true
	}
	return completeNone, 0, 0, "", false
}

func slashToken(text []rune, cursor int) (start, end int, word string, ok bool) {
	if cursor == 0 || text[0] != '/' {
		return 0, 0, "", false
	}
	if len(text) >= 2 && text[0] == '/' && text[1] == '/' {
		return 0, 0, "", false
	}
	for i := 0; i < cursor; i++ {
		if unicode.IsSpace(text[i]) {
			return 0, 0, "", false
		}
	}
	if cursor <= 1 {
		return 0, cursor, "", true
	}
	return 0, cursor, string(text[1:cursor]), true
}

func fileToken(text []rune, cursor int) (start, end int, word string, ok bool) {
	start = cursor
	for start > 0 && !unicode.IsSpace(text[start-1]) {
		start--
	}
	if start >= cursor || text[start] != '@' {
		return 0, 0, "", false
	}
	word = string(text[start+1 : cursor])
	return start, cursor, word, true
}

func slashCompletionItems() []string {
	names := make([]string, 0, len(slashCommands))
	seen := map[string]struct{}{}
	for _, cmd := range slashCommands {
		if _, dup := seen[cmd.name]; dup {
			continue
		}
		seen[cmd.name] = struct{}{}
		names = append(names, cmd.name[1:])
	}
	return names
}

func newSlashCompletion() *completion {
	return &completion{menu: newMenu(slashCompletionItems()), kind: completeSlash}
}

const slashRowPad = 14

func slashRows(names []string) []string {
	rows := make([]string, len(names))
	for i, name := range names {
		full := "/" + name
		rows[i] = full + strings.Repeat(" ", max(slashRowPad-len(full), 1)) + slashSummary(full)
	}
	return rows
}
