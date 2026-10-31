package tui

import (
	"slices"
	"testing"
)

func feed(e *Editor, input string) []Action {
	var d Decoder
	var actions []Action
	keys := append(d.Feed([]byte(input)), d.Flush()...)
	for _, k := range keys {
		if action := e.Handle(k); action != ActionNone {
			actions = append(actions, action)
		}
	}
	return actions
}

func TestEditorKeys(t *testing.T) {
	const left = "\x1b[D"
	const right = "\x1b[C"
	const home = "\x1b[H"
	const end = "\x1b[F"
	const del = "\x1b[3~"
	const backspace = "\x7f"
	tests := []struct {
		name       string
		input      string
		wantText   string
		wantCursor int
	}{
		{"insert", "abc", "abc", 3},
		{"insert wide", "日本", "日本", 2},
		{"insert mid", "ac" + left + "b", "abc", 2},
		{"left stops at start", "a" + left + left, "a", 0},
		{"right stops at end", "a" + right, "a", 1},
		{"right after left", "ab" + left + left + right, "ab", 1},
		{"backspace", "abc" + backspace, "ab", 2},
		{"backspace mid", "abc" + left + backspace, "ac", 1},
		{"backspace at start", "a" + left + backspace, "a", 0},
		{"backspace wide rune", "日本" + backspace, "日", 1},
		{"delete", "abc" + left + del, "ab", 2},
		{"delete at end", "abc" + del, "abc", 3},
		{"home", "abc" + home, "abc", 0},
		{"end", "abc" + home + end, "abc", 3},
		{"ctrl a", "abc\x01", "abc", 0},
		{"ctrl e", "abc\x01\x05", "abc", 3},
		{"ctrl d deletes forward", "abc" + left + "\x04", "ab", 2},
		{"ctrl d at end", "abc\x04", "abc", 3},
		{"ctrl u", "abcd" + left + left + "\x15", "cd", 0},
		{"ctrl k", "abcd" + left + left + "\x0b", "ab", 2},
		{"ctrl w", "one two\x17", "one ", 4},
		{"ctrl w trailing space", "one two  \x17", "one ", 4},
		{"ctrl w mid word", "one two" + left + "\x17", "one o", 4},
		{"ctrl w at start", "a" + left + "\x17", "a", 0},
		{"ctrl w across newline", "\x1b[200~one\n\x1b[201~\x17", "", 0},
		{"home on second line", "\x1b[200~ab\ncd\x1b[201~" + home, "ab\ncd", 3},
		{"end on first line", "\x1b[200~ab\ncd\x1b[201~" + left + left + left + left + end, "ab\ncd", 2},
		{"ctrl u keeps first line", "\x1b[200~ab\ncd\x1b[201~\x15", "ab\n", 3},
		{"ctrl k keeps second line", "\x1b[200~ab\ncd\x1b[201~" + left + left + left + left + "\x0b", "a\ncd", 1},
		{"paste", "\x1b[200~a\tb\x1b[201~", "a\tb", 3},
		{"paste mid buffer", "ad" + left + "\x1b[200~b\nc\x1b[201~", "ab\ncd", 4},
		{"esc ignored", "a\x1b", "a", 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var e Editor
			if actions := feed(&e, tc.input); len(actions) != 0 {
				t.Fatalf("actions %v", actions)
			}
			if e.Text() != tc.wantText || e.Cursor() != tc.wantCursor {
				t.Fatalf("got %q at %d, want %q at %d", e.Text(), e.Cursor(), tc.wantText, tc.wantCursor)
			}
		})
	}
}

func TestEditorActions(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		want     []Action
		wantText string
	}{
		{"enter submits", "hi\r", []Action{ActionSubmit}, "hi"},
		{"line feed submits", "hi\n", []Action{ActionSubmit}, "hi"},
		{"paste with newline does not submit", "\x1b[200~a\nb\n\x1b[201~", nil, "a\nb\n"},
		{"paste carriage return does not submit", "\x1b[200~a\rb\x1b[201~", nil, "a\rb"},
		{"enter after paste submits", "\x1b[200~a\nb\x1b[201~\r", []Action{ActionSubmit}, "a\nb"},
		{"ctrl c keeps text", "hi\x03", []Action{ActionInterrupt}, "hi"},
		{"ctrl d on empty", "\x04", []Action{ActionEOF}, ""},
		{"ctrl d on text", "hi\x04", nil, "hi"},
		{"ctrl l", "hi\x0c", []Action{ActionRedraw}, "hi"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var e Editor
			if got := feed(&e, tc.input); !slices.Equal(got, tc.want) {
				t.Fatalf("got %v want %v", got, tc.want)
			}
			if e.Text() != tc.wantText {
				t.Fatalf("text %q want %q", e.Text(), tc.wantText)
			}
		})
	}
}

func TestEditorHistory(t *testing.T) {
	const up = "\x1b[A"
	const down = "\x1b[B"
	tests := []struct {
		name     string
		entries  []string
		input    string
		wantText string
	}{
		{"up with no history keeps draft", nil, "draft" + up, "draft"},
		{"up recalls newest", []string{"one", "two"}, up, "two"},
		{"up twice recalls older", []string{"one", "two"}, up + up, "one"},
		{"up stops at oldest", []string{"one", "two"}, up + up + up, "one"},
		{"down moves newer", []string{"one", "two"}, up + up + down, "two"},
		{"down restores draft", []string{"one", "two"}, "draft" + up + up + down + down, "draft"},
		{"down restores empty draft", []string{"one"}, up + down, ""},
		{"down at draft stays", []string{"one"}, "draft" + down, "draft"},
		{"edit of recalled line is dropped", []string{"one"}, "draft" + up + "x" + down, "draft"},
		{"history unchanged by edit", []string{"one"}, up + "x" + down + up, "one"},
		{"blank not recorded", []string{"one", ""}, up, "one"},
		{"repeat not recorded", []string{"one", "two", "two"}, up + up, "one"},
		{"multi line entry", []string{"a\nb"}, up, "a\nb"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var e Editor
			for _, entry := range tc.entries {
				feed(&e, "\x1b[200~"+entry+"\x1b[201~")
				e.Take()
			}
			feed(&e, tc.input)
			if e.Text() != tc.wantText {
				t.Fatalf("got %q want %q", e.Text(), tc.wantText)
			}
			if e.Cursor() != len(e.Runes()) {
				t.Fatalf("cursor %d, len %d", e.Cursor(), len(e.Runes()))
			}
		})
	}
}

func TestEditorTake(t *testing.T) {
	var e Editor
	feed(&e, "hello")
	if got := e.Take(); got != "hello" {
		t.Fatalf("take %q", got)
	}
	if e.Text() != "" || e.Cursor() != 0 {
		t.Fatalf("not cleared: %q at %d", e.Text(), e.Cursor())
	}
}

func TestEditorClearKeepsHistory(t *testing.T) {
	var e Editor
	feed(&e, "one")
	e.Take()
	feed(&e, "two\x1b[A")
	e.Clear()
	feed(&e, "\x1b[A")
	if e.Text() != "one" {
		t.Fatalf("got %q", e.Text())
	}
}
