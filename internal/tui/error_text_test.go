package tui

import (
	"errors"
	"strings"
	"testing"
)

func TestShortErrorKeepsTheWholeMessage(t *testing.T) {
	url := "https://opencode.ai/v2/docs/console/go/#where-to-find-the-session-id"
	body := "http 400: {\"error\":{\"message\":\"" + strings.Repeat("filler ", 40) + "See " + url + " for the fix\"}}"
	tests := []struct {
		name string
		err  string
		want string
	}{
		{"long message keeps its url", body, url + " for the fix"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shortError(errors.New(tt.err))
			if !strings.Contains(got, tt.want) {
				t.Fatalf("%q does not contain %q", got, tt.want)
			}
		})
	}
}

func TestErrorTextCutsAtAWordBoundary(t *testing.T) {
	text := strings.Repeat("word ", maxErrorRunes)
	got := strings.TrimPrefix(shortError(errors.New(text)), "error: ")
	if !strings.HasSuffix(got, "word…") {
		t.Fatalf("tail %q", got[len(got)-20:])
	}
	if n := len([]rune(got)); n > maxErrorRunes+1 {
		t.Fatalf("%d runes", n)
	}
}
