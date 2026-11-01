package anthropic

import (
	"encoding/json"
	"errors"
	"iter"
	"slices"

	"github.com/HoneyPaptan/hp-harness/internal/provider"
)

var ErrTruncated = errors.New("stream ended before message_stop")

type pendingToolCall struct {
	id   string
	name string
	seed json.RawMessage
	args string
}

type assembler struct {
	tools      map[int]*pendingToolCall
	stopReason string
	usage      wireUsage
	stopped    bool
}

func streamEvents(events iter.Seq2[wireEvent, error]) iter.Seq2[provider.Event, error] {
	return func(yield func(provider.Event, error) bool) {
		a := &assembler{tools: make(map[int]*pendingToolCall)}
		for ev, err := range events {
			if err != nil {
				yield(provider.Event{}, err)
				return
			}
			for _, out := range a.feed(ev) {
				if !yield(out, nil) {
					return
				}
			}
		}
		if !a.stopped {
			yield(provider.Event{}, &provider.TransientError{Err: ErrTruncated})
			return
		}
		for _, out := range a.finish() {
			if !yield(out, nil) {
				return
			}
		}
	}
}

func (a *assembler) feed(ev wireEvent) []provider.Event {
	switch ev.Type {
	case "message_start":
		a.mergeUsage(ev.Message.Usage)
	case "content_block_start":
		return a.startBlock(ev.Index, ev.ContentBlock)
	case "content_block_delta":
		return a.applyDelta(ev.Index, ev.Delta)
	case "message_delta":
		a.mergeUsage(ev.Usage)
		a.stopReason = ev.Delta.StopReason
	case "message_stop":
		a.stopped = true
	}
	return nil
}

func (a *assembler) mergeUsage(u wireUsage) {
	a.usage.InputTokens = positiveOr(u.InputTokens, a.usage.InputTokens)
	a.usage.OutputTokens = positiveOr(u.OutputTokens, a.usage.OutputTokens)
	a.usage.CacheCreationTokens = positiveOr(u.CacheCreationTokens, a.usage.CacheCreationTokens)
	a.usage.CacheReadTokens = positiveOr(u.CacheReadTokens, a.usage.CacheReadTokens)
}

func positiveOr(v, fallback int) int {
	if v > 0 {
		return v
	}
	return fallback
}

func (a *assembler) startBlock(index int, b wireStartBlock) []provider.Event {
	switch b.Type {
	case "tool_use":
		a.tools[index] = &pendingToolCall{id: b.ID, name: b.Name, seed: b.Input}
	case "text":
		return textEvent(b.Text)
	}
	return nil
}

func (a *assembler) applyDelta(index int, d wireDelta) []provider.Event {
	switch d.Type {
	case "text_delta":
		return textEvent(d.Text)
	case "input_json_delta":
		if p := a.tools[index]; p != nil {
			p.args += d.PartialJSON
		}
	}
	return nil
}

func textEvent(text string) []provider.Event {
	if text == "" {
		return nil
	}
	return []provider.Event{{Kind: provider.EventText, Text: text}}
}

func (a *assembler) finish() []provider.Event {
	out := make([]provider.Event, 0, len(a.tools)+1)
	for _, i := range sortedIndexes(a.tools) {
		out = append(out, provider.Event{Kind: provider.EventToolCall, Call: a.tools[i].call()})
	}
	return append(out, provider.Event{
		Kind: provider.EventDone, Usage: a.totalUsage(), StopReason: a.stopReason,
	})
}

func (p *pendingToolCall) call() provider.ToolCall {
	return provider.ToolCall{ID: p.id, Name: p.name, Args: p.arguments()}
}

func (p *pendingToolCall) arguments() json.RawMessage {
	if p.args != "" {
		return json.RawMessage(p.args)
	}
	if len(p.seed) > 0 {
		return p.seed
	}
	return json.RawMessage(emptyInput)
}

func (a *assembler) totalUsage() provider.Usage {
	u := a.usage
	return provider.Usage{
		InputTokens:  u.InputTokens + u.CacheCreationTokens + u.CacheReadTokens,
		OutputTokens: u.OutputTokens,
	}
}

func sortedIndexes(m map[int]*pendingToolCall) []int {
	out := make([]int, 0, len(m))
	for i := range m {
		out = append(out, i)
	}
	slices.Sort(out)
	return out
}
