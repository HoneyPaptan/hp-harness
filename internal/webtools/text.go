package webtools

import (
	"bytes"
	"strconv"
	"strings"
)

func htmlToText(src []byte) (title, body string) {
	var out bytes.Buffer
	var titleBuf bytes.Buffer
	inPre := false
	skipDepth := 0
	inHead := false
	pendingSpace := false
	lastNewline := false

	i := 0
	for i < len(src) {
		if skipDepth > 0 {
			if tag, end, ok := readTag(src, i); ok {
				if isCloseTag(tag) {
					name := tagName(tag)
					if dropTag(name) {
						skipDepth--
					}
				} else if !isCloseTag(tag) && dropTag(tagName(tag)) && !isSelfClosing(tag) {
					skipDepth++
				}
				i = end
				continue
			}
			i++
			continue
		}
		if inHead {
			if tag, end, ok := readTag(src, i); ok {
				name := tagName(tag)
				if isCloseTag(tag) && name == "head" {
					inHead = false
					i = end
					continue
				}
				if !isCloseTag(tag) && name == "title" {
					inner, next := elementText(src, end, "title")
					titleBuf.WriteString(unescapeEntities(inner))
					i = next
					continue
				}
				i = end
				continue
			}
			i++
			continue
		}
		if tag, end, ok := readTag(src, i); ok {
			name := tagName(tag)
			if isCloseTag(tag) {
				if name == "pre" {
					inPre = false
				}
				if blockTag(name) {
					writeNewline(&out, &pendingSpace, &lastNewline)
				}
				i = end
				continue
			}
			if dropTag(name) {
				if !isSelfClosing(tag) {
					skipDepth = 1
				}
				i = end
				continue
			}
			if name == "head" {
				inHead = true
				i = end
				continue
			}
			if name == "pre" {
				inPre = true
				writeNewline(&out, &pendingSpace, &lastNewline)
				i = end
				continue
			}
			if blockTag(name) {
				writeNewline(&out, &pendingSpace, &lastNewline)
			}
			if name == "br" {
				writeNewline(&out, &pendingSpace, &lastNewline)
			}
			i = end
			continue
		}
		ch := src[i]
		if inPre {
			out.WriteByte(ch)
			lastNewline = ch == '\n'
			pendingSpace = false
			i++
			continue
		}
		if isSpace(ch) {
			pendingSpace = true
			i++
			continue
		}
		if pendingSpace && out.Len() > 0 && !lastNewline {
			out.WriteByte(' ')
		}
		pendingSpace = false
		out.WriteByte(ch)
		lastNewline = false
		i++
	}
	return strings.TrimSpace(titleBuf.String()), normalizeBody(unescapeEntities(out.String()))
}

func unescapeEntities(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	i := 0
	for i < len(s) {
		if s[i] != '&' {
			b.WriteByte(s[i])
			i++
			continue
		}
		next, repl, ok := parseEntity(s, i)
		if !ok {
			b.WriteByte('&')
			i++
			continue
		}
		b.WriteString(repl)
		i = next
	}
	return b.String()
}

func parseEntity(s string, i int) (next int, repl string, ok bool) {
	if i+1 >= len(s) {
		return i, "", false
	}
	if s[i+1] == '#' {
		return parseNumericEntity(s, i)
	}
	return parseNamedEntity(s, i)
}

func parseNumericEntity(s string, i int) (next int, repl string, ok bool) {
	j := i + 2
	if j >= len(s) {
		return i, "", false
	}
	hex := s[j] == 'x' || s[j] == 'X'
	if hex {
		j++
	}
	start := j
	for j < len(s) && j < i+20 {
		ch := s[j]
		if hex {
			if !isHexDigit(ch) {
				break
			}
		} else if ch < '0' || ch > '9' {
			break
		}
		j++
	}
	if start == j || j >= len(s) || s[j] != ';' {
		return i, "", false
	}
	raw := s[start:j]
	var n uint64
	var err error
	if hex {
		n, err = strconv.ParseUint(raw, 16, 32)
	} else {
		n, err = strconv.ParseUint(raw, 10, 32)
	}
	if err != nil {
		return j + 1, "\uFFFD", true
	}
	return j + 1, codePointString(uint32(n)), true
}

func parseNamedEntity(s string, i int) (next int, repl string, ok bool) {
	j := i + 1
	for j < len(s) && isNamedEntityByte(s[j]) {
		j++
	}
	if j == i+1 || j >= len(s) || s[j] != ';' {
		return i, "", false
	}
	name := s[i+1 : j]
	if v, found := lookupNamedEntity(name); found {
		return j + 1, v, true
	}
	return j + 1, s[i : j+1], true
}

func lookupNamedEntity(name string) (string, bool) {
	switch name {
	case "amp":
		return "&", true
	case "lt":
		return "<", true
	case "gt":
		return ">", true
	case "quot":
		return "\"", true
	case "apos":
		return "'", true
	case "nbsp":
		return " ", true
	case "copy":
		return "\u00A9", true
	case "reg":
		return "\u00AE", true
	case "trade":
		return "\u2122", true
	case "hellip":
		return "\u2026", true
	case "mdash":
		return "\u2014", true
	case "ndash":
		return "\u2013", true
	case "lsquo":
		return "\u2018", true
	case "rsquo":
		return "\u2019", true
	case "ldquo":
		return "\u201C", true
	case "rdquo":
		return "\u201D", true
	case "bull":
		return "\u2022", true
	case "middot":
		return "\u00B7", true
	case "laquo":
		return "\u00AB", true
	case "raquo":
		return "\u00BB", true
	case "times":
		return "\u00D7", true
	default:
		return "", false
	}
}

func codePointString(n uint32) string {
	if n > 0x10FFFF || (n >= 0xD800 && n <= 0xDFFF) {
		return "\uFFFD"
	}
	return string(rune(n))
}

func isHexDigit(b byte) bool {
	return (b >= '0' && b <= '9') || (b >= 'a' && b <= 'f') || (b >= 'A' && b <= 'F')
}

func isNamedEntityByte(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

func normalizeBody(s string) string {
	lines := strings.Split(s, "\n")
	var b strings.Builder
	blank := false
	for _, line := range lines {
		trimmed := strings.TrimRight(line, " \t")
		if trimmed == "" {
			if blank {
				continue
			}
			blank = true
			b.WriteByte('\n')
			continue
		}
		blank = false
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(trimmed)
	}
	return strings.TrimSpace(b.String())
}

func writeNewline(out *bytes.Buffer, pendingSpace *bool, lastNewline *bool) {
	if out.Len() == 0 {
		return
	}
	if *lastNewline {
		return
	}
	out.WriteByte('\n')
	*pendingSpace = false
	*lastNewline = true
}

func readTag(src []byte, i int) (tag string, end int, ok bool) {
	if i >= len(src) || src[i] != '<' {
		return "", i, false
	}
	j := i + 1
	for j < len(src) && src[j] != '>' {
		j++
	}
	if j >= len(src) {
		return "", i, false
	}
	return string(src[i : j+1]), j + 1, true
}

func tagName(tag string) string {
	tag = strings.TrimPrefix(tag, "<")
	tag = strings.TrimPrefix(tag, "/")
	tag = strings.TrimSpace(tag)
	if idx := strings.IndexAny(tag, " \t\n\r/>"); idx >= 0 {
		tag = tag[:idx]
	}
	return strings.ToLower(tag)
}

func isCloseTag(tag string) bool {
	return len(tag) > 1 && tag[1] == '/'
}

func isSelfClosing(tag string) bool {
	return strings.HasSuffix(strings.TrimSpace(tag), "/>")
}

func dropTag(name string) bool {
	switch name {
	case "script", "style", "noscript", "svg":
		return true
	default:
		return false
	}
}

func blockTag(name string) bool {
	switch name {
	case "p", "div", "li", "tr", "h1", "h2", "h3", "h4", "h5", "h6",
		"section", "article", "header", "footer", "nav", "table", "ul", "ol", "blockquote":
		return true
	default:
		return false
	}
}

func isSpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r'
}

func elementText(src []byte, start int, name string) (text string, next int) {
	closeTag := []byte("</" + name)
	var b bytes.Buffer
	i := start
	depth := 0
	for i < len(src) {
		if i+len(closeTag) <= len(src) && asciiEqualFold(src[i:i+len(closeTag)], closeTag) {
			if depth == 0 {
				j := i + len(closeTag)
				for j < len(src) && src[j] != '>' {
					j++
				}
				if j < len(src) {
					return b.String(), j + 1
				}
				return b.String(), j
			}
			depth--
		}
		if src[i] == '<' {
			if inner, end, ok := readTag(src, i); ok {
				if !isCloseTag(inner) && tagName(inner) == name {
					depth++
				}
				i = end
				continue
			}
		}
		b.WriteByte(src[i])
		i++
	}
	return b.String(), i
}

func asciiEqualFold(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		aa, bb := a[i], b[i]
		if aa >= 'A' && aa <= 'Z' {
			aa += 'a' - 'A'
		}
		if bb >= 'A' && bb <= 'Z' {
			bb += 'a' - 'A'
		}
		if aa != bb {
			return false
		}
	}
	return true
}
