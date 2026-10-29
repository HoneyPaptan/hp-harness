package core

import (
	"iter"

	"github.com/HoneyPaptan/hp-harness/internal/provider"
)

type Observer struct {
	OnTurnStart  func()
	OnEvent      func(provider.Event)
	OnToolStart  func(provider.ToolCall)
	OnToolResult func(provider.ToolResult)
}

func (o Observer) emitTurnStart() {
	if o.OnTurnStart != nil {
		o.OnTurnStart()
	}
}

func (o Observer) emitEvent(ev provider.Event) {
	if o.OnEvent != nil {
		o.OnEvent(ev)
	}
}

func (o Observer) emitToolStart(c provider.ToolCall) {
	if o.OnToolStart != nil {
		o.OnToolStart(c)
	}
}

func (o Observer) emitToolResult(r provider.ToolResult) {
	if o.OnToolResult != nil {
		o.OnToolResult(r)
	}
}

func tap(seq iter.Seq2[provider.Event, error], fn func(provider.Event)) iter.Seq2[provider.Event, error] {
	return func(yield func(provider.Event, error) bool) {
		for ev, err := range seq {
			if err != nil {
				if !yield(ev, err) {
					return
				}
				continue
			}
			if ev.Kind == provider.EventText {
				fn(ev)
			}
			if !yield(ev, err) {
				return
			}
		}
	}
}
