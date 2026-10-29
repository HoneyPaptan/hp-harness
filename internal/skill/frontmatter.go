package skill

import (
	"strings"
	"unicode"
)

func parseFrontmatter(text string) (map[string]string, string, bool) {
	lines := splitLines(text)
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return nil, text, false
	}
	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			end = i
			break
		}
	}
	if end < 0 {
		return nil, text, false
	}
	fields := parseFieldLines(lines[1:end])
	body := strings.Join(lines[end+1:], "\n")
	return fields, body, true
}

func parseFieldLines(lines []string) map[string]string {
	out := make(map[string]string)
	for i := 0; i < len(lines); {
		line := lines[i]
		if strings.TrimSpace(line) == "" {
			i++
			continue
		}
		if indentLevel(line) > 0 {
			i++
			continue
		}
		key, val, block, ok := parseFieldLine(line)
		if !ok {
			i++
			continue
		}
		i++
		if block != "" {
			val, i = readBlock(lines, i, block)
		}
		out[key] = val
	}
	return out
}

func parseFieldLine(line string) (key, val, block string, ok bool) {
	i := strings.Index(line, ":")
	if i <= 0 {
		return "", "", "", false
	}
	key = strings.TrimSpace(line[:i])
	if key == "" || strings.Contains(key, " ") {
		return "", "", "", false
	}
	rest := strings.TrimSpace(line[i+1:])
	if kind, ok := blockKind(rest); ok {
		return key, "", kind, true
	}
	val, ok = parseScalar(rest)
	if !ok {
		return "", "", "", false
	}
	return key, val, "", true
}

func parseScalar(rest string) (string, bool) {
	if rest == "" {
		return "", true
	}
	if rest[0] == '\'' {
		return parseQuoted(rest, '\'')
	}
	if rest[0] == '"' {
		return parseQuoted(rest, '"')
	}
	return rest, true
}

func blockKind(rest string) (string, bool) {
	switch strings.TrimRight(rest, "-+") {
	case ">", "":
		return ">", true
	case "|":
		return "|", true
	}
	return "", false
}

func parseQuoted(s string, q byte) (string, bool) {
	if len(s) < 2 || s[0] != q || s[len(s)-1] != q {
		return "", false
	}
	return s[1 : len(s)-1], true
}

func readBlock(lines []string, start int, kind string) (string, int) {
	var parts []string
	i := start
	for i < len(lines) {
		line := lines[i]
		if strings.TrimSpace(line) == "" {
			parts = append(parts, "")
			i++
			continue
		}
		if indentLevel(line) == 0 {
			break
		}
		parts = append(parts, trimIndent(line))
		i++
	}
	sep := " "
	if kind == "|" {
		sep = "\n"
	}
	return strings.TrimSpace(strings.Join(parts, sep)), i
}

func indentLevel(line string) int {
	n := 0
	for _, r := range line {
		if r == ' ' {
			n++
			continue
		}
		if r == '\t' {
			n += 4
			continue
		}
		break
	}
	return n
}

func trimIndent(line string) string {
	return strings.TrimLeftFunc(line, unicode.IsSpace)
}

func splitLines(text string) []string {
	if text == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(text, "\n"), "\n")
}
