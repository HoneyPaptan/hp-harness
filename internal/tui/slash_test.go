package tui

import "testing"

func TestHelpListsCommandsThenKeysInOneColumn(t *testing.T) {
	want := map[string]bool{
		"commands": false, "keys": false,
		"  /help                   commands and keys":                                    false,
		"  /permissions MODE       ask, auto or full-access; reset drops session grants": false,
		"  /crew status|report ID  print one crew":                                       false,
		"  Esc Esc                 stop the turn, or clear the draft":                    false,
		"  Ctrl-W Ctrl-U Ctrl-K    delete a word, to the line start, to the line end":    false,
		"  ↑ ↓                     move between lines, then through earlier prompts":     false,
	}
	for _, line := range helpLines {
		if _, ok := want[line]; ok {
			want[line] = true
		}
		if rowWidth(line) > 80 {
			t.Errorf("help line wider than 80 cells: %q", line)
		}
		if line != "" && line != helpCommands && line != helpKeys && line[helpPad+1] != ' ' {
			t.Errorf("help line breaks the column: %q", line)
		}
	}
	for line, seen := range want {
		if !seen {
			t.Errorf("help is missing %q", line)
		}
	}
}

func TestSlashRowsShowTheCommandAndWhatItDoes(t *testing.T) {
	got := slashRows([]string{"help", "permissions"})
	want := []string{"/help         commands and keys", "/permissions  show the permission mode"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("row %d got %q want %q", i, got[i], want[i])
		}
	}
}
