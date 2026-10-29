package core

import (
	"path/filepath"
	"testing"

	"github.com/HoneyPaptan/hp-harness/internal/host"
	"github.com/HoneyPaptan/hp-harness/internal/journal"
	"github.com/HoneyPaptan/hp-harness/internal/provider/fake"
)

func TestSeed(t *testing.T) {
	h := host.NewMemory()
	dir := t.TempDir()
	jpath := filepath.Join(dir, "run.jsonl")
	fp := &fake.Provider{Turns: []fake.Turn{fake.TextTurn("x")}}
	cfg := Config{
		RunID: "s1", Model: "m", System: "sys", Dir: dir,
		JournalPath: jpath, Prompt: "hello", Preset: "p",
	}
	if err := Seed(Deps{Host: h}, cfg); err != nil {
		t.Fatal(err)
	}
	if len(fp.Requests) != 0 {
		t.Fatalf("provider called")
	}
	recs, err := journal.Read(h, jpath)
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 2 {
		t.Fatalf("records %d", len(recs))
	}
	if recs[0].Type != journal.TypeRunStart || recs[1].Type != journal.TypeUser {
		t.Fatalf("types %s %s", recs[0].Type, recs[1].Type)
	}
	st, err := journal.Replay(recs)
	if err != nil {
		t.Fatal(err)
	}
	if st.Start.StepLimit != defaultStepLimit {
		t.Fatalf("step limit %d", st.Start.StepLimit)
	}
}
