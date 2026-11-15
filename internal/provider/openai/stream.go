package openai

import (
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"sort"
	"strings"

	"github.com/HoneyPaptan/hp-harness/internal/provider"
)

var ErrTruncated = errors.New("stream ended before a finish reason")

type pendingToolCall struct {
	id    string
	name  string
	args  string
	extra json.RawMessage
}

type assembler struct {
	tools        map[int]*pendingToolCall
	finishReason string
	usage        provider.Usage
	done         bool
	text         textCallFilter
	output       bool
}

func newAssembler() *assembler {
	return &assembler{tools: make(map[int]*pendingToolCall)}
}

func streamEvents(chunks iter.Seq2[wireStreamChunk, error]) iter.Seq2[provider.Event, error] {
	return func(yield func(provider.Event, error) bool) {
		a := newAssembler()
		for chunk, err := range chunks {
			if err != nil {
				yield(provider.Event{}, err)
				return
			}
			for _, ev := range a.feed(chunk) {
				if !yield(ev, nil) {
					return
				}
			}
		}
		if !a.complete() {
			yield(provider.Event{}, &provider.TransientError{Err: ErrTruncated})
			return
		}
		for _, ev := range a.finish() {
			if !yield(ev, nil) {
				return
			}
		}
	}
}

func (a *assembler) complete() bool {
	if a.finishReason != "" {
		return true
	}
	if a.done && (a.output || len(a.tools) > 0) {
		a.finishReason = "stop"
		return true
	}
	return false
}

func (a *assembler) feed(chunk wireStreamChunk) []provider.Event {
	if chunk.done {
		a.done = true
		return nil
	}
	a.mergeUsage(chunk.Usage)
	var out []provider.Event
	for _, ch := range chunk.Choices {
		if ch.FinishReason != nil && *ch.FinishReason != "" {
			a.finishReason = *ch.FinishReason
		}
		if text := ch.Delta.reasoning(); text != "" {
			a.output = true
			out = append(out, provider.Event{Kind: provider.EventReasoning, Text: text})
		}
		if ch.Delta.Content != "" {
			a.output = true
			if text := a.text.push(string(ch.Delta.Content)); text != "" {
				out = append(out, provider.Event{Kind: provider.EventText, Text: text})
			}
		}
		for _, tc := range ch.Delta.ToolCalls {
			a.mergeToolDelta(tc)
		}
	}
	return out
}

func (d wireStreamDelta) reasoning() string {
	return string(d.Reasoning) + string(d.ReasoningContent)
}

func (a *assembler) mergeUsage(u *wireUsage) {
	if u == nil {
		return
	}
	a.usage.InputTokens = u.PromptTokens
	a.usage.OutputTokens = u.CompletionTokens
}

func (a *assembler) mergeToolDelta(tc wireStreamToolCall) {
	p := a.tools[tc.Index]
	if p == nil {
		p = &pendingToolCall{}
		a.tools[tc.Index] = p
	}
	if tc.ID != "" {
		p.id = tc.ID
	}
	if tc.Function.Name != "" {
		p.name = tc.Function.Name
	}
	if len(tc.ExtraContent) > 0 {
		p.extra = tc.ExtraContent
	}
	p.args += string(tc.Function.Arguments)
}

func (a *assembler) finish() []provider.Event {
	var out []provider.Event
	calls := a.structuredCalls()
	rest, textCalls := a.text.flush(len(calls) == 0)
	if rest != "" {
		out = append(out, provider.Event{Kind: provider.EventText, Text: rest})
	}
	calls = append(calls, textCalls...)
	stamp := now().UnixNano()
	for i, c := range calls {
		if c.ID == "" {
			c.ID = fmt.Sprintf("call_%x_%d", stamp, i)
		}
		out = append(out, provider.Event{Kind: provider.EventToolCall, Call: c})
	}
	reason := a.finishReason
	if len(textCalls) > 0 {
		reason = "tool_calls"
	}
	return append(out, provider.Event{Kind: provider.EventDone, Usage: a.usage, StopReason: reason})
}

func (a *assembler) structuredCalls() []provider.ToolCall {
	idxs := sortedToolIndexes(a.tools)
	out := make([]provider.ToolCall, 0, len(idxs))
	for _, i := range idxs {
		p := a.tools[i]
		out = append(out, provider.ToolCall{ID: p.id, Name: p.name, Args: normalizeArgs(p.args), Extra: p.extra})
	}
	return out
}

func normalizeArgs(args string) json.RawMessage {
	args = strings.TrimSpace(args)
	if args == "" || args == "null" {
		return json.RawMessage("{}")
	}
	return json.RawMessage(args)
}

func sortedToolIndexes(m map[int]*pendingToolCall) []int {
	idxs := make([]int, 0, len(m))
	for i := range m {
		idxs = append(idxs, i)
	}
	sort.Ints(idxs)
	return idxs
}

func decodeChunk(data string) (wireStreamChunk, error) {
	var chunk wireStreamChunk
	err := json.Unmarshal([]byte(data), &chunk)
	return chunk, err
}
