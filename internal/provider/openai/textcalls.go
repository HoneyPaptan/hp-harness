package openai

import (
	"encoding/json"
	"strings"

	"github.com/HoneyPaptan/hp-harness/internal/provider"
)

const (
	callOpen  = "<tool_call>"
	callClose = "</tool_call>"
)

type textCallFilter struct {
	held    string
	calling bool
	buf     strings.Builder
}

func (f *textCallFilter) push(s string) string {
	if f.calling {
		f.buf.WriteString(s)
		return ""
	}
	s = f.held + s
	f.held = ""
	if i := strings.Index(s, callOpen); i >= 0 {
		f.calling = true
		f.buf.WriteString(s[i:])
		return s[:i]
	}
	keep := partialSuffix(s, callOpen)
	f.held = s[len(s)-keep:]
	return s[:len(s)-keep]
}

func (f *textCallFilter) flush(parse bool) (string, []provider.ToolCall) {
	rest := f.held
	f.held = ""
	if !f.calling {
		return rest, nil
	}
	raw := f.buf.String()
	if parse {
		if calls, tail, ok := parseTextCalls(raw); ok {
			return rest + tail, calls
		}
	}
	return rest + raw, nil
}

func partialSuffix(s, marker string) int {
	for n := min(len(marker)-1, len(s)); n > 0; n-- {
		if strings.HasSuffix(s, marker[:n]) {
			return n
		}
	}
	return 0
}

type textCall struct {
	Name       string          `json:"name"`
	Arguments  json.RawMessage `json:"arguments"`
	Parameters json.RawMessage `json:"parameters"`
}

func parseTextCalls(raw string) ([]provider.ToolCall, string, bool) {
	var calls []provider.ToolCall
	var tail strings.Builder
	for raw != "" {
		start := strings.Index(raw, callOpen)
		if start < 0 {
			tail.WriteString(raw)
			break
		}
		tail.WriteString(raw[:start])
		body := raw[start+len(callOpen):]
		end := strings.Index(body, callClose)
		raw = ""
		if end >= 0 {
			raw = body[end+len(callClose):]
			body = body[:end]
		}
		call, ok := decodeTextCall(body)
		if !ok {
			return nil, "", false
		}
		calls = append(calls, call)
	}
	return calls, strings.TrimSpace(tail.String()), len(calls) > 0
}

func decodeTextCall(body string) (provider.ToolCall, bool) {
	var c textCall
	if err := json.Unmarshal([]byte(strings.TrimSpace(body)), &c); err != nil || c.Name == "" {
		return provider.ToolCall{}, false
	}
	args := c.Arguments
	if len(args) == 0 {
		args = c.Parameters
	}
	var s string
	if json.Unmarshal(args, &s) == nil {
		args = json.RawMessage(s)
	}
	return provider.ToolCall{Name: c.Name, Args: normalizeArgs(string(args))}, true
}
