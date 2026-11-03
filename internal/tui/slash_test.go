package tui

import "testing"

func TestSlashHelpLinesUnchanged(t *testing.T) {
	want := []string{
		"/help          this list",
		"/model         pick a provider and model",
		"/model p/name  switch the model for following turns",
		"/jev           show the jev state",
		"/jev on|off    switch jev for following turns",
		"/key NAME      store an API key",
		"/new           start a fresh run",
		"/runs          list recent runs",
		"/permissions   show or set permission mode",
		"/permissions ask|auto|full-access|reset switch mode or reset session grants",
		"/quit          leave, also Ctrl-D",
		"//text         send /text to the model",
		"Ctrl-C or Esc stops a turn",
	}
	if len(helpLines) != len(want) {
		t.Fatalf("len %d want %d", len(helpLines), len(want))
	}
	for i := range want {
		if helpLines[i] != want[i] {
			t.Fatalf("line %d got %q want %q", i, helpLines[i], want[i])
		}
	}
}
