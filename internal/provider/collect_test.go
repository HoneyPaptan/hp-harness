package provider

import (
	"bytes"
	"encoding/json"
	"errors"
	"iter"
	"testing"
)

func toolCallEqual(a, b ToolCall) bool {
	return a.ID == b.ID && a.Name == b.Name && bytes.Equal(a.Args, b.Args)
}

func messageEmpty(m Message) bool {
	return m.Role == "" && m.Text == "" && len(m.ToolCalls) == 0 && len(m.ToolResults) == 0
}

func seqFrom(steps []struct {
	ev  Event
	err error
}) iter.Seq2[Event, error] {
	return func(yield func(Event, error) bool) {
		for _, s := range steps {
			if !yield(s.ev, s.err) {
				return
			}
			if s.err != nil {
				return
			}
		}
	}
}

func TestCollect(t *testing.T) {
	callA := ToolCall{ID: "a", Name: "read", Args: json.RawMessage(`{"path":"x"}`)}
	callB := ToolCall{ID: "b", Name: "write", Args: json.RawMessage(`{}`)}
	streamErr := errors.New("stream failed")

	tests := []struct {
		name  string
		steps []struct {
			ev  Event
			err error
		}
		wantMsg Message
		wantUse Usage
		wantErr error
	}{
		{
			name: "text only",
			steps: []struct {
				ev  Event
				err error
			}{
				{ev: Event{Kind: EventText, Text: "hel"}},
				{ev: Event{Kind: EventText, Text: "lo"}},
				{ev: Event{Kind: EventDone, StopReason: "end_turn"}},
			},
			wantMsg: Message{Role: RoleAssistant, Text: "hello"},
		},
		{
			name: "text plus two tool calls",
			steps: []struct {
				ev  Event
				err error
			}{
				{ev: Event{Kind: EventText, Text: "go"}},
				{ev: Event{Kind: EventToolCall, Call: callA}},
				{ev: Event{Kind: EventToolCall, Call: callB}},
				{ev: Event{Kind: EventDone, StopReason: "tool_use"}},
			},
			wantMsg: Message{
				Role:      RoleAssistant,
				Text:      "go",
				ToolCalls: []ToolCall{callA, callB},
			},
		},
		{
			name: "error mid turn",
			steps: []struct {
				ev  Event
				err error
			}{
				{ev: Event{Kind: EventText, Text: "part"}},
				{ev: Event{}, err: streamErr},
			},
			wantErr: streamErr,
		},
		{
			name: "missing done",
			steps: []struct {
				ev  Event
				err error
			}{
				{ev: Event{Kind: EventText, Text: "x"}},
			},
			wantErr: ErrNoDone,
		},
		{
			name: "usage from done",
			steps: []struct {
				ev  Event
				err error
			}{
				{ev: Event{Kind: EventText, Text: "ok"}},
				{ev: Event{Kind: EventDone, Usage: Usage{InputTokens: 10, OutputTokens: 3}}},
			},
			wantMsg: Message{Role: RoleAssistant, Text: "ok"},
			wantUse: Usage{InputTokens: 10, OutputTokens: 3},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, use, err := Collect(seqFrom(tt.steps))
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) && err != tt.wantErr {
					t.Fatalf("err = %v, want %v", err, tt.wantErr)
				}
				if !messageEmpty(msg) {
					t.Fatalf("msg = %+v, want empty", msg)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if msg.Role != tt.wantMsg.Role || msg.Text != tt.wantMsg.Text {
				t.Fatalf("msg = %+v, want %+v", msg, tt.wantMsg)
			}
			if len(msg.ToolCalls) != len(tt.wantMsg.ToolCalls) {
				t.Fatalf("tool calls = %+v, want %+v", msg.ToolCalls, tt.wantMsg.ToolCalls)
			}
			for i := range msg.ToolCalls {
				if !toolCallEqual(msg.ToolCalls[i], tt.wantMsg.ToolCalls[i]) {
					t.Fatalf("tool call %d = %+v, want %+v", i, msg.ToolCalls[i], tt.wantMsg.ToolCalls[i])
				}
			}
			if use != tt.wantUse {
				t.Fatalf("usage = %+v, want %+v", use, tt.wantUse)
			}
		})
	}
}
