package fake

import (
	"context"
	"errors"
	"iter"
	"sync"

	"github.com/HoneyPaptan/hp-harness/internal/provider"
)

var ErrScriptDone = errors.New("fake provider script exhausted")

type Turn struct {
	Events []provider.Event
	Err    error
}

type Provider struct {
	Turns    []Turn
	Requests []provider.Request
	mu       sync.Mutex
	next     int
}

func TextTurn(text string) Turn {
	return Turn{
		Events: []provider.Event{
			{Kind: provider.EventText, Text: text},
			{Kind: provider.EventDone, StopReason: "end_turn"},
		},
	}
}

func ToolTurn(calls ...provider.ToolCall) Turn {
	events := make([]provider.Event, 0, len(calls)+1)
	for _, c := range calls {
		events = append(events, provider.Event{Kind: provider.EventToolCall, Call: c})
	}
	events = append(events, provider.Event{Kind: provider.EventDone, StopReason: "tool_use"})
	return Turn{Events: events}
}

func (p *Provider) Stream(ctx context.Context, req provider.Request) iter.Seq2[provider.Event, error] {
	turn, ok := p.takeTurn(req)
	return func(yield func(provider.Event, error) bool) {
		if !ok {
			yield(provider.Event{}, ErrScriptDone)
			return
		}
		for _, ev := range turn.Events {
			if err := ctx.Err(); err != nil {
				yield(provider.Event{}, err)
				return
			}
			if !yield(ev, nil) {
				return
			}
		}
		if turn.Err != nil {
			yield(provider.Event{}, turn.Err)
		}
	}
}

func (p *Provider) takeTurn(req provider.Request) (Turn, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.Requests = append(p.Requests, copyRequest(req))
	if p.next >= len(p.Turns) {
		return Turn{}, false
	}
	turn := p.Turns[p.next]
	p.next++
	return turn, true
}

func copyRequest(req provider.Request) provider.Request {
	out := req
	out.Messages = copyMessages(req.Messages)
	return out
}

func copyMessages(msgs []provider.Message) []provider.Message {
	if msgs == nil {
		return nil
	}
	out := make([]provider.Message, len(msgs))
	for i, m := range msgs {
		out[i] = m
		if m.ToolCalls != nil {
			out[i].ToolCalls = append([]provider.ToolCall(nil), m.ToolCalls...)
		}
		if m.ToolResults != nil {
			out[i].ToolResults = append([]provider.ToolResult(nil), m.ToolResults...)
		}
	}
	return out
}
