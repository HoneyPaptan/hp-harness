package journal

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/HoneyPaptan/hp-harness/internal/host"
	"github.com/HoneyPaptan/hp-harness/internal/provider"
)

func compactStart(t *testing.T, h *host.Memory, fixed time.Time) *Writer {
	t.Helper()
	h.Clock = fixed
	start := RunStart{RunID: "r1", Model: "m", System: "sys", Dir: "/w", StepLimit: 10}
	w, err := Create(h, "/j", start)
	if err != nil {
		t.Fatal(err)
	}
	return w
}

func msgUser(text string) provider.Message {
	return provider.Message{Role: provider.RoleUser, Text: text}
}

func msgAsst(text string) provider.Message {
	return provider.Message{Role: provider.RoleAssistant, Text: text}
}

func TestReplayCompact(t *testing.T) {
	fixed := time.Date(2026, 3, 15, 10, 0, 0, 0, time.UTC)
	h := host.NewMemory()
	h.Clock = fixed
	w := compactStart(t, h, fixed)

	for _, text := range []string{"a", "b", "c", "d", "e"} {
		if err := w.User(text); err != nil {
			t.Fatal(err)
		}
		if err := w.Assistant(msgAsst("r-" + text)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Compact("handoff1", 3); err != nil {
		t.Fatal(err)
	}
	recs, err := Read(h, "/j")
	if err != nil {
		t.Fatal(err)
	}
	st, err := Replay(recs)
	if err != nil {
		t.Fatal(err)
	}
	want := []provider.Message{
		msgUser("handoff1"),
		msgAsst("r-d"),
		msgUser("e"),
		msgAsst("r-e"),
	}
	if !reflect.DeepEqual(st.Messages, want) {
		t.Fatalf("messages\n got %#v\nwant %#v", st.Messages, want)
	}
}

func TestReplayCompactTwice(t *testing.T) {
	fixed := time.Date(2026, 3, 15, 10, 0, 0, 0, time.UTC)
	h := host.NewMemory()
	h.Clock = fixed
	w := compactStart(t, h, fixed)

	if err := w.User("u1"); err != nil {
		t.Fatal(err)
	}
	if err := w.Assistant(msgAsst("a1")); err != nil {
		t.Fatal(err)
	}
	if err := w.User("u2"); err != nil {
		t.Fatal(err)
	}
	if err := w.Assistant(msgAsst("a2")); err != nil {
		t.Fatal(err)
	}
	if err := w.Compact("h1", 2); err != nil {
		t.Fatal(err)
	}
	if err := w.User("u3"); err != nil {
		t.Fatal(err)
	}
	if err := w.Assistant(msgAsst("a3")); err != nil {
		t.Fatal(err)
	}
	if err := w.Compact("h2", 1); err != nil {
		t.Fatal(err)
	}
	recs, err := Read(h, "/j")
	if err != nil {
		t.Fatal(err)
	}
	st, err := Replay(recs)
	if err != nil {
		t.Fatal(err)
	}
	want := []provider.Message{
		msgUser("h2"),
		msgAsst("a3"),
	}
	if !reflect.DeepEqual(st.Messages, want) {
		t.Fatalf("messages\n got %#v\nwant %#v", st.Messages, want)
	}
}

func TestReplayCompactKeepClamped(t *testing.T) {
	fixed := time.Date(2026, 3, 15, 10, 0, 0, 0, time.UTC)
	h := host.NewMemory()
	h.Clock = fixed
	w := compactStart(t, h, fixed)
	if err := w.User("only"); err != nil {
		t.Fatal(err)
	}
	if err := w.Compact("h", 100); err != nil {
		t.Fatal(err)
	}
	recs, _ := Read(h, "/j")
	st, err := Replay(recs)
	if err != nil {
		t.Fatal(err)
	}
	want := []provider.Message{msgUser("h"), msgUser("only")}
	if !reflect.DeepEqual(st.Messages, want) {
		t.Fatalf("messages %#v", st.Messages)
	}
}

func TestReplayCompactNegativeKeep(t *testing.T) {
	fixed := time.Date(2026, 3, 15, 10, 0, 0, 0, time.UTC)
	h := host.NewMemory()
	h.Clock = fixed
	w := compactStart(t, h, fixed)
	if err := w.Compact("h", -1); err != nil {
		t.Fatal(err)
	}
	recs, _ := Read(h, "/j")
	_, err := Replay(recs)
	if err == nil || !strings.Contains(err.Error(), "compact seq") {
		t.Fatalf("err = %v", err)
	}
}

func TestReplayCompactThenAppend(t *testing.T) {
	fixed := time.Date(2026, 3, 15, 10, 0, 0, 0, time.UTC)
	h := host.NewMemory()
	h.Clock = fixed
	w := compactStart(t, h, fixed)
	if err := w.User("before"); err != nil {
		t.Fatal(err)
	}
	if err := w.Compact("h", 1); err != nil {
		t.Fatal(err)
	}
	if err := w.User("after"); err != nil {
		t.Fatal(err)
	}
	recs, _ := Read(h, "/j")
	st, err := Replay(recs)
	if err != nil {
		t.Fatal(err)
	}
	want := []provider.Message{msgUser("h"), msgUser("before"), msgUser("after")}
	if !reflect.DeepEqual(st.Messages, want) {
		t.Fatalf("messages %#v", st.Messages)
	}
}

func TestReplayCompactPendingTail(t *testing.T) {
	fixed := time.Date(2026, 3, 15, 10, 0, 0, 0, time.UTC)
	h := host.NewMemory()
	h.Clock = fixed
	w := compactStart(t, h, fixed)
	oldCall := provider.ToolCall{ID: "old", Name: "read", Args: json.RawMessage(`{}`)}
	tailCall := provider.ToolCall{ID: "tail", Name: "bash", Args: json.RawMessage(`{}`)}
	if err := w.User("go"); err != nil {
		t.Fatal(err)
	}
	if err := w.Assistant(provider.Message{Role: provider.RoleAssistant, ToolCalls: []provider.ToolCall{oldCall}}); err != nil {
		t.Fatal(err)
	}
	if err := w.ToolStarted(oldCall); err != nil {
		t.Fatal(err)
	}
	if err := w.User("more"); err != nil {
		t.Fatal(err)
	}
	if err := w.Assistant(provider.Message{Role: provider.RoleAssistant, ToolCalls: []provider.ToolCall{tailCall}}); err != nil {
		t.Fatal(err)
	}
	if err := w.ToolStarted(tailCall); err != nil {
		t.Fatal(err)
	}
	if err := w.Compact("h", 2); err != nil {
		t.Fatal(err)
	}
	recs, _ := Read(h, "/j")
	st, err := Replay(recs)
	if err != nil {
		t.Fatal(err)
	}
	want := []Pending{{Call: tailCall, Started: true}}
	if !reflect.DeepEqual(st.Pending, want) {
		t.Fatalf("pending %#v", st.Pending)
	}
}
