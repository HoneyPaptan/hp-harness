package cli

import (
	"bytes"
	"path"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/HoneyPaptan/hp-harness/internal/host"
)

const (
	mentionMaxFileBytes  = 64 * 1024
	mentionMaxTotalBytes = 256 * 1024
	mentionBinaryProbe   = 8000
)

var mentionTrailPunct = ",.;:!?)"

func expandMentions(h host.Host, dir, prompt string) string {
	paths := mentionPaths(prompt)
	if len(paths) == 0 {
		return prompt
	}
	var b strings.Builder
	b.WriteString(prompt)
	total := 0
	for _, rel := range paths {
		abs, display, ok := resolveMention(h, dir, rel)
		if !ok {
			continue
		}
		data, err := h.ReadFile(abs)
		if err != nil {
			continue
		}
		if mentionBinary(data) {
			continue
		}
		body, size := trimMentionFile(data)
		if total+size > mentionMaxTotalBytes {
			b.WriteString("\n\n[skipped @")
			b.WriteString(display)
			b.WriteString(": message attachment limit]")
			continue
		}
		total += size
		b.WriteString(formatMentionBlock(display, body))
	}
	return b.String()
}

func mentionPaths(prompt string) []string {
	var out []string
	seen := map[string]bool{}
	for _, w := range strings.Fields(prompt) {
		if !strings.HasPrefix(w, "@") || w == "@" {
			continue
		}
		rel := strings.TrimRight(w[1:], mentionTrailPunct)
		if rel == "" {
			continue
		}
		key := filepath.ToSlash(path.Clean(rel))
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, rel)
	}
	return out
}

func resolveMention(h host.Host, dir, rel string) (abs, display string, ok bool) {
	if rel == "" || filepath.IsAbs(rel) {
		return "", "", false
	}
	display = filepath.ToSlash(path.Clean(rel))
	if mentionBadPath(display) {
		return "", "", false
	}
	parts := mentionParts(display)
	if len(parts) == 0 {
		return "", "", false
	}
	cur := dir
	for i, p := range parts {
		cur = filepath.Join(cur, p)
		ent, err := h.Lstat(cur)
		if err != nil {
			return "", "", false
		}
		if ent.IsLink {
			return "", "", false
		}
		last := i == len(parts)-1
		if ent.IsDir {
			if last {
				return "", "", false
			}
			continue
		}
		if !last {
			return "", "", false
		}
	}
	return cur, display, true
}

func mentionBadPath(clean string) bool {
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return true
	}
	for _, p := range strings.Split(clean, "/") {
		if p == ".." {
			return true
		}
	}
	return false
}

func mentionParts(clean string) []string {
	raw := strings.Split(clean, "/")
	out := make([]string, 0, len(raw))
	for _, p := range raw {
		if p != "" && p != "." {
			out = append(out, p)
		}
	}
	return out
}

func mentionBinary(data []byte) bool {
	n := len(data)
	if n > mentionBinaryProbe {
		n = mentionBinaryProbe
	}
	return bytes.IndexByte(data[:n], 0) >= 0
}

func trimMentionFile(data []byte) (string, int) {
	if len(data) <= mentionMaxFileBytes {
		return string(data), len(data)
	}
	body := string(data[:mentionMaxFileBytes]) + "\n[truncated at 64 KB]"
	return body, mentionMaxFileBytes
}

func formatMentionBlock(displayPath, content string) string {
	fence := mentionFence(content)
	var b strings.Builder
	b.WriteString("\n\n<file path=\"")
	b.WriteString(displayPath)
	b.WriteString("\">\n")
	b.WriteString(fence)
	b.WriteByte('\n')
	b.WriteString(content)
	if content != "" && !strings.HasSuffix(content, "\n") {
		b.WriteByte('\n')
	}
	b.WriteString(fence)
	b.WriteString("\n</file>")
	return b.String()
}

func mentionFence(content string) string {
	n := 3
	for line := range strings.SplitSeq(content, "\n") {
		if !strings.HasPrefix(line, "`") {
			continue
		}
		i := 0
		for i < len(line) && line[i] == '`' {
			i++
		}
		if i >= 3 && (i == len(line) || unicode.IsSpace(rune(line[i]))) {
			if i >= n {
				n = i + 1
			}
		}
	}
	return strings.Repeat("`", n)
}
