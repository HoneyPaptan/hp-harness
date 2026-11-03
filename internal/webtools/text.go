package webtools

import (
	"bytes"
	"html"
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
					titleBuf.WriteString(html.UnescapeString(inner))
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
	return strings.TrimSpace(titleBuf.String()), normalizeBody(html.UnescapeString(out.String()))
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
		if i+len(closeTag) <= len(src) && bytes.EqualFold(src[i:i+len(closeTag)], closeTag) {
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
