package journal

import (
	"encoding/json"
	"errors"
	"io/fs"
	"strings"
	"testing"
	"time"

	"github.com/HoneyPaptan/hp-harness/internal/host"
	"github.com/HoneyPaptan/hp-harness/internal/provider"
)

type appendFailHost struct {
	host.Host
	err error
}

func (a *appendFailHost) Append(path string, line []byte) error {
	if a.err != nil {
		return a.err
	}
	return a.Host.Append(path, line)
}

func TestWriter(t *testing.T) {
	fixed := time.Date(2026, 3, 15, 10, 0, 0, 0, time.UTC)
	h := host.NewMemory()
	h.Clock = fixed
	path := "/run.journal"
	start := RunStart{
		RunID: "r1", Model: "m", System: "s", Dir: "/w", StepLimit: 10,
		Preset: "xai", BaseURL: "https://api.example", KeyEnv: "XAI_API_KEY", MaxTokens: 8192,
	}

	w, err := Create(h, path, start)
	if err != nil {
		t.Fatal(err)
	}
	h.Clock = fixed.Add(time.Second)
	if err := w.User("hi"); err != nil {
		t.Fatal(err)
	}
	h.Clock = fixed.Add(2 * time.Second)
	if err := w.Assistant(provider.Message{Role: provider.RoleAssistant, Text: "ok"}); err != nil {
		t.Fatal(err)
	}

	raw, err := h.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var recs []Record
	for _, line := range strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n") {
		var rec Record
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatal(err)
		}
		recs = append(recs, rec)
	}
	if len(recs) != 3 {
		t.Fatalf("len %d", len(recs))
	}
	for i, wantSeq := range []int{0, 1, 2} {
		if recs[i].Seq != wantSeq {
			t.Fatalf("rec %d seq %d", i, recs[i].Seq)
		}
	}
	if !recs[0].TS.Equal(fixed) {
		t.Fatalf("ts0 %v", recs[0].TS)
	}
	if !recs[1].TS.Equal(fixed.Add(time.Second)) {
		t.Fatalf("ts1 %v", recs[1].TS)
	}
	if !recs[2].TS.Equal(fixed.Add(2 * time.Second)) {
		t.Fatalf("ts2 %v", recs[2].TS)
	}
	if recs[0].Type != TypeRunStart || recs[1].Type != TypeUser || recs[2].Type != TypeAssistant {
		t.Fatal("types")
	}
	var got RunStart
	if err := json.Unmarshal(recs[0].Data, &got); err != nil {
		t.Fatal(err)
	}
	if got != start {
		t.Fatalf("run_start %+v", got)
	}

	h2 := host.NewMemory()
	h2.Clock = fixed
	if err := h2.WriteFile(path, []byte("x")); err != nil {
		t.Fatal(err)
	}
	if _, err := Create(h2, path, start); err == nil {
		t.Fatal("expected error")
	} else if !errors.Is(err, fs.ErrExist) {
		t.Fatalf("got %v", err)
	}

	h3 := host.NewMemory()
	h3.Clock = fixed
	fail := &appendFailHost{Host: h3}
	w3, err := Create(fail, "/x.journal", start)
	if err != nil {
		t.Fatal(err)
	}
	if w3.seq != 1 {
		t.Fatalf("seq after create %d", w3.seq)
	}
	fail.err = errors.New("boom")
	if err := w3.User("x"); err == nil {
		t.Fatal("expected append error")
	}
	if w3.seq != 1 {
		t.Fatalf("seq after failed append %d", w3.seq)
	}
}
