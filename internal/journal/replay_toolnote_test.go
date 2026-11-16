package journal

import (
	"testing"

	"github.com/HoneyPaptan/hp-harness/internal/host"
	"github.com/HoneyPaptan/hp-harness/internal/provider"
)

func TestReplayToolNotes(t *testing.T) {
	h := host.NewMemory()
	w, err := Create(h, "/j", RunStart{RunID: "r1", Model: "m", Dir: "/w", StepLimit: 10})
	if err != nil {
		t.Fatal(err)
	}
	steps := []struct {
		name      string
		write     func() error
		wantNotes int
		wantSteps int
		wantLast  provider.Role
	}{
		{"prompt", func() error { return w.User("go") }, 0, 0, provider.RoleUser},
		{"first note", func() error { return w.ToolNote("one") }, 1, 0, provider.RoleUser},
		{"second note", func() error { return w.ToolNote("two") }, 2, 0, provider.RoleUser},
		{"assistant resets", func() error { return w.Assistant(provider.Message{Role: provider.RoleAssistant, Text: "ok"}) }, 0, 1, provider.RoleAssistant},
		{"note after", func() error { return w.ToolNote("three") }, 1, 1, provider.RoleUser},
		{"new prompt resets", func() error { return w.User("again") }, 0, 0, provider.RoleUser},
	}
	for _, s := range steps {
		if err := s.write(); err != nil {
			t.Fatal(err)
		}
		_, recs, err := Open(h, "/j")
		if err != nil {
			t.Fatal(err)
		}
		st, err := Replay(recs)
		if err != nil {
			t.Fatal(err)
		}
		last := st.Messages[len(st.Messages)-1]
		if st.ToolNotes != s.wantNotes || st.Steps != s.wantSteps || last.Role != s.wantLast {
			t.Fatalf("%s: notes %d steps %d last %+v", s.name, st.ToolNotes, st.Steps, last)
		}
	}
}
