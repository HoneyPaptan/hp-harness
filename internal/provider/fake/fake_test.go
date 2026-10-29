package fake

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/HoneyPaptan/hp-harness/internal/provider"
)

func toolCallEqual(a, b provider.ToolCall) bool {
	return a.ID == b.ID && a.Name == b.Name && bytes.Equal(a.Args, b.Args)
}

func collectEvents(seq func(yield func(provider.Event, error) bool)) ([]provider.Event, error) {
	var events []provider.Event
	var streamErr error
	for ev, err := range seq {
		if err != nil {
			streamErr = err
			break
		}
		events = append(events, ev)
	}
	return events, streamErr
}

func messagesEqual(a, b []provider.Message) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Role != b[i].Role || a[i].Text != b[i].Text {
			return false
		}
		if len(a[i].ToolCalls) != len(b[i].ToolCalls) || len(a[i].ToolResults) != len(b[i].ToolResults) {
			return false
		}
		for j := range a[i].ToolCalls {
			if !toolCallEqual(a[i].ToolCalls[j], b[i].ToolCalls[j]) {
				return false
			}
		}
		for j := range a[i].ToolResults {
			if a[i].ToolResults[j] != b[i].ToolResults[j] {
				return false
			}
		}
	}
	return true
}

func TestProvider(t *testing.T) {
	call := provider.ToolCall{ID: "1", Name: "read", Args: json.RawMessage(`{}`)}
	turnErr := errors.New("turn failed")

	tests := []struct {
		name string
		run  func(t *testing.T)
	}{
		{
			name: "plays turns in order",
			run: func(t *testing.T) {
				p := &Provider{Turns: []Turn{TextTurn("a"), TextTurn("b")}}
				ev1, err := collectEvents(p.Stream(context.Background(), provider.Request{}))
				if err != nil {
					t.Fatal(err)
				}
				if len(ev1) != 2 || ev1[0].Text != "a" {
					t.Fatalf("first turn: %+v", ev1)
				}
				ev2, err := collectEvents(p.Stream(context.Background(), provider.Request{}))
				if err != nil {
					t.Fatal(err)
				}
				if len(ev2) != 2 || ev2[0].Text != "b" {
					t.Fatalf("second turn: %+v", ev2)
				}
			},
		},
		{
			name: "records request copy",
			run: func(t *testing.T) {
				p := &Provider{Turns: []Turn{TextTurn("x")}}
				msgs := []provider.Message{{
					Role:        provider.RoleUser,
					Text:        "hi",
					ToolResults: []provider.ToolResult{{CallID: "c", Text: "out"}},
				}}
				req := provider.Request{Model: "m", Messages: msgs}
				_, _ = collectEvents(p.Stream(context.Background(), req))
				msgs[0].Text = "mutated"
				msgs[0].ToolResults[0].Text = "changed"
				if !messagesEqual(p.Requests[0].Messages, []provider.Message{{
					Role:        provider.RoleUser,
					Text:        "hi",
					ToolResults: []provider.ToolResult{{CallID: "c", Text: "out"}},
				}}) {
					t.Fatalf("recorded %+v", p.Requests[0].Messages)
				}
			},
		},
		{
			name: "error turn after events",
			run: func(t *testing.T) {
				p := &Provider{Turns: []Turn{
					{Events: []provider.Event{{Kind: provider.EventText, Text: "x"}}, Err: turnErr},
				}}
				_, err := collectEvents(p.Stream(context.Background(), provider.Request{}))
				if !errors.Is(err, turnErr) {
					t.Fatalf("err = %v", err)
				}
			},
		},
		{
			name: "exhausted script",
			run: func(t *testing.T) {
				p := &Provider{Turns: []Turn{TextTurn("once")}}
				_, _ = collectEvents(p.Stream(context.Background(), provider.Request{}))
				_, err := collectEvents(p.Stream(context.Background(), provider.Request{}))
				if !errors.Is(err, ErrScriptDone) {
					t.Fatalf("err = %v", err)
				}
			},
		},
		{
			name: "cancelled context",
			run: func(t *testing.T) {
				p := &Provider{Turns: []Turn{TextTurn("slow")}}
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				_, err := collectEvents(p.Stream(ctx, provider.Request{}))
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("err = %v", err)
				}
			},
		},
		{
			name: "early break advances turn",
			run: func(t *testing.T) {
				p := &Provider{Turns: []Turn{TextTurn("first"), TextTurn("second")}}
				seq := p.Stream(context.Background(), provider.Request{})
				for ev, err := range seq {
					if err != nil {
						t.Fatal(err)
					}
					if ev.Kind == provider.EventText {
						break
					}
				}
				ev2, err := collectEvents(p.Stream(context.Background(), provider.Request{}))
				if err != nil {
					t.Fatal(err)
				}
				if len(ev2) < 1 || ev2[0].Text != "second" {
					t.Fatalf("second turn after break: %+v", ev2)
				}
			},
		},
		{
			name: "tool turn",
			run: func(t *testing.T) {
				p := &Provider{Turns: []Turn{ToolTurn(call)}}
				evs, err := collectEvents(p.Stream(context.Background(), provider.Request{}))
				if err != nil {
					t.Fatal(err)
				}
				if len(evs) != 2 || evs[0].Call.ID != "1" || evs[1].StopReason != "tool_use" {
					t.Fatalf("evs = %+v", evs)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.run(t)
		})
	}
}
