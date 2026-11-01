package journal

import (
	"testing"
	"time"

	"github.com/HoneyPaptan/hp-harness/internal/host"
	"github.com/HoneyPaptan/hp-harness/internal/provider"
)

func TestMarkDetachedAndStop(t *testing.T) {
	fixed := time.Date(2026, 3, 15, 10, 0, 0, 0, time.UTC)
	h := host.NewMemory()
	h.Clock = fixed
	path := "/marks.journal"
	if _, err := Create(h, path, RunStart{RunID: "r1", Model: "m", System: "s", Dir: "/w", StepLimit: 10}); err != nil {
		t.Fatal(err)
	}
	markWriter, _, err := Open(h, path)
	if err != nil {
		t.Fatal(err)
	}
	if err := markWriter.Detached(); err != nil {
		t.Fatal(err)
	}
	if err := MarkStop(h, path, "user"); err != nil {
		t.Fatal(err)
	}
	recs, err := Read(h, path)
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 3 {
		t.Fatalf("len %d", len(recs))
	}
	if recs[1].Type != TypeDetached || recs[2].Type != TypeRunStop {
		t.Fatalf("types %s %s", recs[1].Type, recs[2].Type)
	}
	for i, want := range []int{0, 1, 2} {
		if recs[i].Seq != want {
			t.Fatalf("rec %d seq %d", i, recs[i].Seq)
		}
	}
	st, err := Replay(recs)
	if err != nil {
		t.Fatal(err)
	}
	if !st.Detached || !st.Stopped {
		t.Fatalf("detached=%v stopped=%v", st.Detached, st.Stopped)
	}
	if len(st.Messages) != 0 {
		t.Fatalf("marks leaked into history %#v", st.Messages)
	}
}

func TestMarkStopKeepsRunReplayable(t *testing.T) {
	fixed := time.Date(2026, 3, 15, 10, 0, 0, 0, time.UTC)
	h := host.NewMemory()
	h.Clock = fixed
	path := "/stop-resumable.journal"
	if _, err := Create(h, path, RunStart{RunID: "r1", Model: "m", System: "s", Dir: "/w", StepLimit: 10}); err != nil {
		t.Fatal(err)
	}
	w, _, err := Open(h, path)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.User("task"); err != nil {
		t.Fatal(err)
	}
	if err := w.Assistant(provider.Message{Role: provider.RoleAssistant, Text: "half"}); err != nil {
		t.Fatal(err)
	}
	if err := MarkStop(h, path, "user"); err != nil {
		t.Fatal(err)
	}
	recs, err := Read(h, path)
	if err != nil {
		t.Fatal(err)
	}
	st, err := Replay(recs)
	if err != nil {
		t.Fatal(err)
	}
	if st.Finished || firstUser(st) != "task" || lastText(st) != "half" {
		t.Fatal("stop mark broke the replay")
	}
	if !st.Stopped || st.Detached {
		t.Fatalf("detached=%v stopped=%v", st.Detached, st.Stopped)
	}
}

func TestNewUserTurnClearsStopMark(t *testing.T) {
	h := host.NewMemory()
	h.Clock = time.Date(2026, 3, 15, 10, 0, 0, 0, time.UTC)
	path := "/resume.journal"
	if _, err := Create(h, path, RunStart{RunID: "r1", Model: "m", System: "s", Dir: "/w", StepLimit: 10}); err != nil {
		t.Fatal(err)
	}
	if err := MarkStop(h, path, "user"); err != nil {
		t.Fatal(err)
	}
	w, _, err := Open(h, path)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.User("again"); err != nil {
		t.Fatal(err)
	}
	recs, _ := Read(h, path)
	st, err := Replay(recs)
	if err != nil {
		t.Fatal(err)
	}
	if st.Stopped {
		t.Fatal("stop mark survived a new user turn")
	}
}

func firstUser(st State) string {
	for _, m := range st.Messages {
		if m.Role == provider.RoleUser {
			return m.Text
		}
	}
	return ""
}

func lastText(st State) string {
	msgs := st.Messages
	if len(msgs) == 0 {
		return ""
	}
	return msgs[len(msgs)-1].Text
}
