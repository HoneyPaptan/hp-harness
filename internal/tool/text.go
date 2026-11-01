package tool

import "fmt"

func Truncate(s string) string {
	return truncateMiddle(s, MaxOutput)
}

func truncateHead(s string, max int) string {
	if len(s) <= max {
		return s
	}
	cut := utf8SafeCut(s, max)
	omitted := len(s) - cut
	return s[:cut] + fmt.Sprintf("\n[truncated: %d bytes omitted]", omitted)
}

func truncateMiddle(s string, max int) string {
	if len(s) <= max {
		return s
	}
	note := fmt.Sprintf("\n[truncated: %d bytes omitted]\n", len(s)-max)
	budget := max - len(note)
	if budget < 2 {
		return truncateHead(s, max)
	}
	half := budget / 2
	head := s[:utf8SafeCut(s, half)]
	tailStart := len(s) - utf8SafeCutReverse(s, budget-half)
	return head + note + s[tailStart:]
}

func utf8SafeCut(s string, max int) int {
	cut := max
	if cut > len(s) {
		cut = len(s)
	}
	for cut > 0 && cut < len(s) && s[cut]&0xC0 == 0x80 {
		cut--
	}
	return cut
}

func utf8SafeCutReverse(s string, want int) int {
	start := len(s) - want
	if start < 0 {
		start = 0
	}
	for start < len(s) && s[start]&0xC0 == 0x80 {
		start++
	}
	return len(s) - start
}
