package openai

import (
	"encoding/json"
	"errors"
	"iter"
	"sort"

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
		if a.finishReason == "" {
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

func (a *assembler) feed(chunk wireStreamChunk) []provider.Event {
	a.mergeUsage(chunk.Usage)
	var out []provider.Event
	for _, ch := range chunk.Choices {
		if ch.FinishReason != nil && *ch.FinishReason != "" {
			a.finishReason = *ch.FinishReason
		}
		if ch.Delta.Content != "" {
			out = append(out, provider.Event{Kind: provider.EventText, Text: string(ch.Delta.Content)})
		}
		for _, tc := range ch.Delta.ToolCalls {
			a.mergeToolDelta(tc)
		}
	}
	return out
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
	p.args += tc.Function.Arguments
}

func (a *assembler) finish() []provider.Event {
	idxs := sortedToolIndexes(a.tools)
	out := make([]provider.Event, 0, len(idxs)+1)
	for _, i := range idxs {
		p := a.tools[i]
		args := p.args
		if args == "" {
			args = "{}"
		}
		out = append(out, provider.Event{
			Kind: provider.EventToolCall,
			Call: provider.ToolCall{ID: p.id, Name: p.name, Args: json.RawMessage(args), Extra: p.extra},
		})
	}
	out = append(out, provider.Event{
		Kind:       provider.EventDone,
		Usage:      a.usage,
		StopReason: a.finishReason,
	})
	return out
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
