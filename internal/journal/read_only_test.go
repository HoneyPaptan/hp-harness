package journal

import (
	"bytes"
	"testing"

	"github.com/HoneyPaptan/hp-harness/internal/host"
)

func TestReadTornTailLeavesFileUntouched(t *testing.T) {
	h := host.NewMemory()
	w, err := Create(h, "/j.jsonl", RunStart{RunID: "r"})
	if err != nil {
		t.Fatal(err)
	}
	if err := w.User("hi"); err != nil {
		t.Fatal(err)
	}
	if err := h.Append("/j.jsonl", []byte(`{"seq":2,"ts":"x","type":"tool_result","data":{"res`)); err != nil {
		t.Fatal(err)
	}
	before, _ := h.ReadFile("/j.jsonl")
	recs, err := Read(h, "/j.jsonl")
	if err != nil || len(recs) != 2 {
		t.Fatalf("recs %d err %v", len(recs), err)
	}
	after, _ := h.ReadFile("/j.jsonl")
	if !bytes.Equal(before, after) {
		t.Fatal("Read rewrote the journal")
	}
}
