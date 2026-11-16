package journal

import (
	"bytes"
	"testing"
	"time"

	"github.com/HoneyPaptan/hp-harness/internal/host"
	"github.com/HoneyPaptan/hp-harness/internal/provider"
)

type countingHost struct {
	host.Host
	appends int
}

func (c *countingHost) Append(path string, line []byte) error {
	c.appends++
	return c.Host.Append(path, line)
}

func TestCreateWithUserWritesOneAppendWithTheSameBytes(t *testing.T) {
	fixed := time.Date(2026, 3, 15, 10, 0, 0, 0, time.UTC)
	start := RunStart{RunID: "r1", Model: "m", Dir: "/w", StepLimit: 10}
	seq := host.NewMemory()
	seq.Clock = fixed
	w, err := Create(seq, "/run.journal", start)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.User("hi"); err != nil {
		t.Fatal(err)
	}
	want, err := seq.ReadFile("/run.journal")
	if err != nil {
		t.Fatal(err)
	}

	mem := host.NewMemory()
	mem.Clock = fixed
	counted := &countingHost{Host: mem}
	w, err = CreateWithUser(counted, "/run.journal", start, "hi")
	if err != nil {
		t.Fatal(err)
	}
	got, err := mem.ReadFile("/run.journal")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
	if counted.appends != 1 {
		t.Fatalf("appends = %d, want 1", counted.appends)
	}
	if err := w.Assistant(provider.Message{Role: provider.RoleAssistant, Text: "ok"}); err != nil {
		t.Fatal(err)
	}
	recs, err := Read(mem, "/run.journal")
	if err != nil || len(recs) != 3 || recs[2].Seq != 2 {
		t.Fatalf("records %+v, err %v", recs, err)
	}
}

func TestCreateWithUserRefusesAnExistingJournal(t *testing.T) {
	mem := host.NewMemory()
	if err := mem.WriteFile("/run.journal", []byte("x")); err != nil {
		t.Fatal(err)
	}
	if _, err := CreateWithUser(mem, "/run.journal", RunStart{}, "hi"); err == nil {
		t.Fatal("expected an error")
	}
}
