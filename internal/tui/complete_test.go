package tui

import "testing"

func TestTokenAt(t *testing.T) {
	tests := []struct {
		name      string
		text      string
		cursor    int
		wantKind  int
		wantStart int
		wantEnd   int
		wantWord  string
		wantOK    bool
	}{
		{"slash empty", "/", 1, completeSlash, 0, 1, "", true},
		{"slash partial", "/mo", 3, completeSlash, 0, 3, "mo", true},
		{"slash with space closed", "/model x", 7, completeNone, 0, 0, "", false},
		{"double slash closed", "//x", 3, completeNone, 0, 0, "", false},
		{"file in text", "hi @in", 6, completeFile, 3, 6, "in", true},
		{"file bare at", "hi @", 4, completeFile, 3, 4, "", true},
		{"file after space closed", "@a b", 4, completeNone, 0, 0, "", false},
		{"email not file", "a@b", 2, completeNone, 0, 0, "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			text := []rune(tc.text)
			if tc.cursor < 0 {
				tc.cursor = len(text)
			}
			kind, start, end, word, ok := tokenAt(text, tc.cursor)
			if ok != tc.wantOK || kind != tc.wantKind || start != tc.wantStart || end != tc.wantEnd || word != tc.wantWord {
				t.Fatalf("got kind=%d start=%d end=%d word=%q ok=%v want kind=%d start=%d end=%d word=%q ok=%v",
					kind, start, end, word, ok, tc.wantKind, tc.wantStart, tc.wantEnd, tc.wantWord, tc.wantOK)
			}
		})
	}
}
