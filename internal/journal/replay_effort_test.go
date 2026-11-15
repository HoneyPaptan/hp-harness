package journal

import (
	"testing"

	"github.com/HoneyPaptan/hp-harness/internal/host"
)

func TestReplayEffortFromStartThenSwitch(t *testing.T) {
	h := host.NewMemory()
	w, err := Create(h, "/j", RunStart{RunID: "r1", Model: "m", Dir: "/w", StepLimit: 10, Effort: "low"})
	if err != nil {
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
	if st.Effort != "low" {
		t.Fatalf("start effort %q", st.Effort)
	}
	if err := w.EffortSwitch("high"); err != nil {
		t.Fatal(err)
	}
	_, recs, err = Open(h, "/j")
	if err != nil {
		t.Fatal(err)
	}
	if st, err = Replay(recs); err != nil {
		t.Fatal(err)
	}
	if st.Effort != "high" {
		t.Fatalf("switched effort %q", st.Effort)
	}
}
