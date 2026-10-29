package openai

import (
	"encoding/json"

	"github.com/HoneyPaptan/hp-harness/internal/provider"
)

func buildRequest(req provider.Request) wireRequest {
	w := wireRequest{Model: req.Model}
	w.Messages = appendMessages(w.Messages, req)
	w.Tools = mapTools(req.Tools)
	w.MaxTokens = mapMaxTokens(req.MaxTokens)
	return w
}

func appendMessages(out []wireMessage, req provider.Request) []wireMessage {
	if req.System != "" {
		out = append(out, systemMessage(req.System))
	}
	for _, m := range req.Messages {
		out = appendMessage(out, m)
	}
	return out
}

func systemMessage(text string) wireMessage {
	return wireMessage{Role: "system", Content: wireString(text)}
}

func appendMessage(out []wireMessage, m provider.Message) []wireMessage {
	switch m.Role {
	case provider.RoleUser:
		return appendUser(out, m)
	case provider.RoleAssistant:
		return append(out, assistantMessage(m))
	default:
		return out
	}
}

func appendUser(out []wireMessage, m provider.Message) []wireMessage {
	for _, tr := range m.ToolResults {
		out = append(out, toolResultMessage(tr))
	}
	if m.Text != "" {
		out = append(out, wireMessage{Role: "user", Content: wireString(m.Text)})
	}
	return out
}

func toolResultMessage(tr provider.ToolResult) wireMessage {
	text := tr.Text
	if tr.IsError {
		text = "error: " + text
	}
	return wireMessage{
		Role:       "tool",
		Content:    wireString(text),
		ToolCallID: tr.CallID,
	}
}

func assistantMessage(m provider.Message) wireMessage {
	wm := wireMessage{Role: "assistant", Content: assistantContent(m)}
	if len(m.ToolCalls) > 0 {
		wm.ToolCalls = mapAssistantToolCalls(m.ToolCalls)
	}
	return wm
}

func assistantContent(m provider.Message) wireContent {
	if m.Text == "" && len(m.ToolCalls) > 0 {
		return wireContent{null: true}
	}
	return wireString(m.Text)
}

func wireString(s string) wireContent {
	return wireContent{text: &s}
}

func mapAssistantToolCalls(calls []provider.ToolCall) []wireToolCall {
	out := make([]wireToolCall, len(calls))
	for i, c := range calls {
		out[i] = wireToolCall{
			ID:   c.ID,
			Type: "function",
			Function: wireFunction{
				Name:      c.Name,
				Arguments: toolCallArguments(c.Args),
			},
		}
	}
	return out
}

func toolCallArguments(args json.RawMessage) string {
	if len(args) == 0 {
		return "{}"
	}
	return string(args)
}

func mapTools(tools []provider.ToolDef) []wireTool {
	if len(tools) == 0 {
		return nil
	}
	out := make([]wireTool, len(tools))
	for i, t := range tools {
		out[i] = wireTool{
			Type: "function",
			Function: wireFunction{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.Schema,
			},
		}
	}
	return out
}

func mapMaxTokens(n int) *int {
	if n == 0 {
		return nil
	}
	return &n
}
