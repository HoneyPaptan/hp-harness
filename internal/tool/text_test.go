package tool

import (
	"strings"
	"testing"
)

func TestTruncateHead(t *testing.T) {
	tests := []struct {
		name string
		s    string
		max  int
		want string
	}{
		{"passthrough", "hello", 10, "hello"},
		{"cut", strings.Repeat("a", 20), 10, strings.Repeat("a", 10) + "\n[truncated: 10 bytes omitted]"},
		{"multibyte", "日本語テスト", 7, "日本" + "\n[truncated: 12 bytes omitted]"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := truncateHead(tc.s, tc.max)
			if got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}

func TestTruncateMiddle(t *testing.T) {
	long := strings.Repeat("x", 100)
	tests := []struct {
		name  string
		s     string
		max   int
		check func(string) bool
	}{
		{"passthrough", "short", 100, func(got string) bool { return got == "short" }},
		{"head tail", long, 60, func(got string) bool {
			mark := "\n[truncated:"
			i := strings.Index(got, mark)
			if i < 0 {
				return false
			}
			j := strings.Index(got[i:], "]\n")
			if j < 0 {
				return false
			}
			head, tail := got[:i], got[i+j+2:]
			return strings.HasPrefix(long, head) && strings.HasSuffix(long, tail) && head != tail
		}},
		{"multibyte", "αβγδεζηθ", 12, func(got string) bool {
			return strings.Contains(got, "[truncated:") && strings.HasPrefix(got, "αβ")
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := truncateMiddle(tc.s, tc.max)
			if !tc.check(got) {
				t.Fatalf("unexpected %q", got)
			}
		})
	}
}
