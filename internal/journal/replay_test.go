package journal

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/HoneyPaptan/hp-harness/internal/host"
	"github.com/HoneyPaptan/hp-harness/internal/provider"
)

func TestReplay(t *testing.T) {
	fixed := time.Date(2026, 3, 15, 10, 0, 0, 0, time.UTC)
	start := RunStart{RunID: "r1", Model: "m", System: "sys", Dir: "/w", StepLimit: 10}

	writeTwoSteps := func(t *testing.T) []Record {
		t.Helper()
		h := host.NewMemory()
		h.Clock = fixed
		w, err := Create(h, "/j", start)
		if err != nil {
			t.Fatal(err)
		}
		if err := w.User("go"); err != nil {
			t.Fatal(err)
		}
		c1 := provider.ToolCall{ID: "c1", Name: "read", Args: json.RawMessage(`{"path":"a"}`)}
		c2 := provider.ToolCall{ID: "c2", Name: "bash", Args: json.RawMessage(`{"cmd":"x"}`)}
		if err := w.Assistant(provider.Message{Role: provider.RoleAssistant, ToolCalls: []provider.ToolCall{c1, c2}}); err != nil {
			t.Fatal(err)
		}
		if err := w.ToolStarted(c1); err != nil {
			t.Fatal(err)
		}
		if err := w.ToolStarted(c2); err != nil {
			t.Fatal(err)
		}
		if err := w.ToolResult(provider.ToolResult{CallID: "c1", Text: "out1"}); err != nil {
			t.Fatal(err)
		}
		if err := w.ToolResult(provider.ToolResult{CallID: "c2", Text: "out2"}); err != nil {
			t.Fatal(err)
		}
		if err := w.Assistant(provider.Message{Role: provider.RoleAssistant, Text: "done"}); err != nil {
			t.Fatal(err)
		}
		if err := w.End("complete"); err != nil {
			t.Fatal(err)
		}
		recs, err := Read(h, "/j")
		if err != nil {
			t.Fatal(err)
		}
		return recs
	}

	t.Run("two full steps", func(t *testing.T) {
		recs := writeTwoSteps(t)
		st, err := Replay(recs)
		if err != nil {
			t.Fatal(err)
		}
		want := []provider.Message{
			{Role: provider.RoleUser, Text: "go"},
			{Role: provider.RoleAssistant, ToolCalls: []provider.ToolCall{
				{ID: "c1", Name: "read", Args: json.RawMessage(`{"path":"a"}`)},
				{ID: "c2", Name: "bash", Args: json.RawMessage(`{"cmd":"x"}`)},
			}},
			{Role: provider.RoleUser, ToolResults: []provider.ToolResult{
				{CallID: "c1", Text: "out1"},
				{CallID: "c2", Text: "out2"},
			}},
			{Role: provider.RoleAssistant, Text: "done"},
		}
		if !reflect.DeepEqual(st.Messages, want) {
			t.Fatalf("messages\n got %#v\nwant %#v", st.Messages, want)
		}
		if st.Steps != 2 {
			t.Fatalf("steps %d", st.Steps)
		}
		if !st.Finished || st.EndReason != "complete" {
			t.Fatalf("finished %v reason %q", st.Finished, st.EndReason)
		}
		if len(st.Pending) != 0 {
			t.Fatalf("pending %v", st.Pending)
		}
	})

	t.Run("crash after tool_started", func(t *testing.T) {
		h := host.NewMemory()
		h.Clock = fixed
		w, err := Create(h, "/j", start)
		if err != nil {
			t.Fatal(err)
		}
		c := provider.ToolCall{ID: "c1", Name: "bash", Args: json.RawMessage(`{}`)}
		if err := w.User("x"); err != nil {
			t.Fatal(err)
		}
		if err := w.Assistant(provider.Message{Role: provider.RoleAssistant, ToolCalls: []provider.ToolCall{c}}); err != nil {
			t.Fatal(err)
		}
		if err := w.ToolStarted(c); err != nil {
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
		want := []Pending{{Call: c, Started: true}}
		if !reflect.DeepEqual(st.Pending, want) {
			t.Fatalf("pending %#v", st.Pending)
		}
		if st.Finished {
			t.Fatal("finished")
		}
	})

	t.Run("pending not started", func(t *testing.T) {
		h := host.NewMemory()
		h.Clock = fixed
		w, err := Create(h, "/j", start)
		if err != nil {
			t.Fatal(err)
		}
		c1 := provider.ToolCall{ID: "a", Name: "read", Args: json.RawMessage(`{}`)}
		c2 := provider.ToolCall{ID: "b", Name: "read", Args: json.RawMessage(`{}`)}
		if err := w.Assistant(provider.Message{Role: provider.RoleAssistant, ToolCalls: []provider.ToolCall{c1, c2}}); err != nil {
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
		want := []Pending{
			{Call: c1, Started: false},
			{Call: c2, Started: false},
		}
		if !reflect.DeepEqual(st.Pending, want) {
			t.Fatalf("pending %#v", st.Pending)
		}
	})

	t.Run("missing run_start", func(t *testing.T) {
		rec := Record{Seq: 0, Type: TypeUser, Data: json.RawMessage(`{"text":"x"}`)}
		_, err := Replay([]Record{rec})
		if !errors.Is(err, ErrNoRunStart) {
			t.Fatalf("err %v", err)
		}
	})

	t.Run("empty journal", func(t *testing.T) {
		_, err := Replay(nil)
		if !errors.Is(err, ErrEmpty) {
			t.Fatalf("err %v", err)
		}
	})

	t.Run("attach result merges", func(t *testing.T) {
		msgs := []provider.Message{{Role: provider.RoleUser, ToolResults: []provider.ToolResult{{CallID: "a"}}}}
		msgs = AttachResult(msgs, provider.ToolResult{CallID: "b"})
		if len(msgs) != 1 || len(msgs[0].ToolResults) != 2 {
			t.Fatalf("msgs %#v", msgs)
		}
	})

	t.Run("unknown type", func(t *testing.T) {
		recs := writeTwoSteps(t)
		recs = append(recs, Record{Seq: 99, Type: Type("nope"), Data: json.RawMessage(`{}`)})
		if _, err := Replay(recs); err == nil || !strings.Contains(err.Error(), "unknown") {
			t.Fatalf("err %v", err)
		}
	})

	t.Run("model switch sets current", func(t *testing.T) {
		h := host.NewMemory()
		h.Clock = fixed
		w, err := Create(h, "/j", start)
		if err != nil {
			t.Fatal(err)
		}
		if err := w.ModelSwitch("groq/a", "groq/b"); err != nil {
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
		if st.Current != "groq/b" {
			t.Fatalf("current %q", st.Current)
		}
	})

	t.Run("old journal current empty", func(t *testing.T) {
		recs := writeTwoSteps(t)
		st, err := Replay(recs)
		if err != nil {
			t.Fatal(err)
		}
		if st.Current != "" {
			t.Fatalf("current %q", st.Current)
		}
	})

	t.Run("limit wait unchanged messages", func(t *testing.T) {
		h := host.NewMemory()
		h.Clock = fixed
		w, err := Create(h, "/j", start)
		if err != nil {
			t.Fatal(err)
		}
		if err := w.User("hi"); err != nil {
			t.Fatal(err)
		}
		until := fixed.Add(30 * time.Second)
		if err := w.LimitWait("groq/m", until); err != nil {
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
		want := []provider.Message{{Role: provider.RoleUser, Text: "hi"}}
		if !reflect.DeepEqual(st.Messages, want) {
			t.Fatalf("messages %#v", st.Messages)
		}
		if st.Steps != 0 {
			t.Fatalf("steps %d", st.Steps)
		}
	})
}

func TestReplayContinuation(t *testing.T) {
	assistant := provider.Message{Role: provider.RoleAssistant, Text: "a"}
	call := provider.ToolCall{ID: "same", Name: "read"}
	withCall := provider.Message{Role: provider.RoleAssistant, ToolCalls: []provider.ToolCall{call}}
	finish := func(w *Writer) error {
		if err := w.User("one"); err != nil {
			return err
		}
		if err := w.Assistant(assistant); err != nil {
			return err
		}
		return w.End("final")
	}
	tests := []struct {
		name         string
		write        func(*Writer) error
		wantFinished bool
		wantReason   string
		wantSteps    int
		wantPending  int
	}{
		{"finished", finish, true, "final", 1, 0},
		{"user after run_end opens the run", func(w *Writer) error {
			if err := finish(w); err != nil {
				return err
			}
			return w.User("two")
		}, false, "", 0, 0},
		{"second run_end closes it again", func(w *Writer) error {
			if err := finish(w); err != nil {
				return err
			}
			if err := w.User("two"); err != nil {
				return err
			}
			if err := w.Assistant(assistant); err != nil {
				return err
			}
			return w.End("step_limit")
		}, true, "step_limit", 1, 0},
		{"reused call id stays pending", func(w *Writer) error {
			if err := w.User("one"); err != nil {
				return err
			}
			if err := w.Assistant(withCall); err != nil {
				return err
			}
			if err := w.ToolStarted(call); err != nil {
				return err
			}
			if err := w.ToolResult(provider.ToolResult{CallID: "same", Text: "x"}); err != nil {
				return err
			}
			return w.Assistant(withCall)
		}, false, "", 2, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := host.NewMemory()
			w, err := Create(h, "/j", RunStart{RunID: "r"})
			if err != nil {
				t.Fatal(err)
			}
			if err := tt.write(w); err != nil {
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
			if st.Finished != tt.wantFinished || st.EndReason != tt.wantReason || st.Steps != tt.wantSteps {
				t.Fatalf("state %+v", st)
			}
			if len(st.Pending) != tt.wantPending {
				t.Fatalf("pending %+v", st.Pending)
			}
			if tt.wantPending > 0 && st.Pending[0].Started {
				t.Fatalf("pending %+v", st.Pending)
			}
		})
	}
}
