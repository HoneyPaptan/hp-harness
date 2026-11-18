package tui

import "testing"

const (
	undoKey     = "\x1f"
	redoKey     = "\x1b_"
	shiftLeft   = "\x1b[1;2D"
	shiftRight  = "\x1b[1;2C"
	shiftHome   = "\x1b[1;2H"
	shiftUp     = "\x1b[1;2A"
	shiftWordL  = "\x1b[1;6D"
	copyKey     = "\x1bw"
	pasteOpen   = "\x1b[200~"
	pasteClose  = "\x1b[201~"
	plainLeft   = "\x1b[D"
	backspaceIn = "\x7f"
)

func TestEditorUndoRedo(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"undo takes back one word", "hello world" + undoKey, "hello"},
		{"undo twice empties the draft", "hello world" + undoKey + undoKey, ""},
		{"undo with nothing to undo", undoKey, ""},
		{"redo brings the word back", "hello world" + undoKey + redoKey, "hello world"},
		{"typing after undo drops redo", "hello world" + undoKey + "!" + redoKey, "hello!"},
		{"undo a kill", "one two\x17" + undoKey, "one two"},
		{"undo a backspace", "abc" + backspaceIn + undoKey, "abc"},
		{"a paste is one step", "a " + pasteOpen + "one two three" + pasteClose + undoKey, "a "},
		{"moving the cursor splits typing", "ab" + plainLeft + "c" + undoKey, "ab"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var e Editor
			feed(&e, tt.input)
			if e.Text() != tt.want {
				t.Fatalf("text %q want %q", e.Text(), tt.want)
			}
		})
	}
}

func TestEditorDiscardCanBeUndone(t *testing.T) {
	var e Editor
	feed(&e, "keep me")
	e.Discard()
	if e.Text() != "" {
		t.Fatalf("discard left %q", e.Text())
	}
	feed(&e, undoKey)
	if e.Text() != "keep me" {
		t.Fatalf("undo gave %q", e.Text())
	}
}

func TestEditorClearForgetsUndo(t *testing.T) {
	var e Editor
	feed(&e, "secret")
	e.Clear()
	feed(&e, undoKey)
	if e.Text() != "" {
		t.Fatalf("a cleared draft came back: %q", e.Text())
	}
}

func TestEditorTakeForgetsUndo(t *testing.T) {
	var e Editor
	feed(&e, "sent")
	e.Take()
	feed(&e, undoKey)
	if e.Text() != "" {
		t.Fatalf("a sent draft came back: %q", e.Text())
	}
}

func TestEditorSelection(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantText   string
		wantCursor int
		wantFrom   int
		wantTo     int
		wantMarked bool
	}{
		{"shift left marks one rune", "abc" + shiftLeft, "abc", 2, 2, 3, true},
		{"shift home marks the line", "abc" + shiftHome, "abc", 0, 0, 3, true},
		{"shift word marks a word", "one two" + shiftWordL, "one two", 4, 4, 7, true},
		{"shift up marks to the line above", "ab\ncd" + shiftUp, "ab\ncd", 2, 2, 5, true},
		{"shift up on the first line marks to the start", "abc" + shiftUp, "abc", 0, 0, 3, true},
		{"back and forth marks nothing", "abc" + shiftLeft + shiftRight, "abc", 3, 0, 0, false},
		{"a plain move drops the mark", "abc" + shiftLeft + plainLeft, "abc", 1, 0, 0, false},
		{"typing replaces the selection", "abc" + shiftLeft + shiftLeft + "x", "ax", 2, 0, 0, false},
		{"backspace removes only the selection", "abc" + shiftLeft + shiftLeft + backspaceIn, "a", 1, 0, 0, false},
		{"ctrl w cuts the selection", "one two" + shiftWordL + "\x17", "one ", 4, 0, 0, false},
		{"undo restores a replaced selection", "abc" + shiftLeft + "x" + undoKey, "abc", 2, 0, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var e Editor
			feed(&e, tt.input)
			from, to, marked := e.Selection()
			if e.Text() != tt.wantText || e.Cursor() != tt.wantCursor {
				t.Fatalf("text %q cursor %d, want %q %d", e.Text(), e.Cursor(), tt.wantText, tt.wantCursor)
			}
			if marked != tt.wantMarked || from != tt.wantFrom || to != tt.wantTo {
				t.Fatalf("selection %d %d %v, want %d %d %v", from, to, marked, tt.wantFrom, tt.wantTo, tt.wantMarked)
			}
		})
	}
}

func TestEditorCopyAndCut(t *testing.T) {
	var e Editor
	actions := feed(&e, "one two"+shiftWordL+copyKey)
	if len(actions) != 1 || actions[0] != ActionCopy || e.Killed() != "two" || e.Text() != "one two" {
		t.Fatalf("copy: actions %v killed %q text %q", actions, e.Killed(), e.Text())
	}
	actions = feed(&e, "\x17")
	if len(actions) != 1 || actions[0] != ActionCopy || e.Text() != "one " {
		t.Fatalf("cut: actions %v text %q", actions, e.Text())
	}
	if actions = feed(&e, copyKey); len(actions) != 0 {
		t.Fatalf("copy with no selection acted: %v", actions)
	}
}

func TestPromptViewMarksSelection(t *testing.T) {
	view := promptView("› ", []rune("abcd"), 1, 80, span{1, 3})
	if want := "› a" + reverseOn + "bc" + reverseOff + "d"; view.rows[0] != want {
		t.Fatalf("row %q want %q", view.rows[0], want)
	}
	view = promptView("› ", []rune("ab\ncd"), 0, 80, span{0, 5})
	if want := "› " + reverseOn + "ab" + reverseOff; view.rows[0] != want {
		t.Fatalf("first row %q want %q", view.rows[0], want)
	}
	if want := "  " + reverseOn + "cd" + reverseOff; view.rows[1] != want {
		t.Fatalf("second row %q want %q", view.rows[1], want)
	}
	if plain := promptView("› ", []rune("abcd"), 1, 80, span{}); plain.rows[0] != "› abcd" {
		t.Fatalf("unmarked row %q", plain.rows[0])
	}
}
