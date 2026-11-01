package openai

import (
	"encoding/json"
	"strings"
)

type wireRequest struct {
	Model         string             `json:"model"`
	Messages      []wireMessage      `json:"messages"`
	Tools         []wireTool         `json:"tools,omitempty"`
	Stream        bool               `json:"stream"`
	StreamOptions *wireStreamOptions `json:"stream_options,omitempty"`
	MaxTokens     *int               `json:"max_tokens,omitempty"`
	Thinking      *wireThinking      `json:"thinking,omitempty"`
}

type wireThinking struct {
	Type string `json:"type"`
}

type wireStreamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

type wireMessage struct {
	Role       string         `json:"role"`
	Content    wireContent    `json:"content"`
	ToolCalls  []wireToolCall `json:"tool_calls,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
}

type wireContent struct {
	text *string
	null bool
}

func (c wireContent) MarshalJSON() ([]byte, error) {
	if c.null {
		return []byte("null"), nil
	}
	if c.text == nil {
		return []byte(`""`), nil
	}
	return json.Marshal(*c.text)
}

type wireToolCall struct {
	ID           string          `json:"id"`
	Type         string          `json:"type"`
	Function     wireFunction    `json:"function"`
	ExtraContent json.RawMessage `json:"extra_content,omitempty"`
}

type wireFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
	Arguments   string          `json:"arguments,omitempty"`
}

type wireTool struct {
	Type     string       `json:"type"`
	Function wireFunction `json:"function"`
}

type wireStreamChunk struct {
	Choices []wireStreamChoice `json:"choices"`
	Usage   *wireUsage         `json:"usage,omitempty"`
	Error   *wireStreamError   `json:"error,omitempty"`
}

type wireStreamChoice struct {
	Delta        wireStreamDelta `json:"delta"`
	FinishReason *string         `json:"finish_reason"`
}

type wireStreamDelta struct {
	Content   wireText             `json:"content"`
	ToolCalls []wireStreamToolCall `json:"tool_calls"`
}

type wireStreamToolCall struct {
	Index        int             `json:"index"`
	ID           string          `json:"id"`
	Function     wireFunction    `json:"function"`
	ExtraContent json.RawMessage `json:"extra_content"`
}

type wireUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}

type wireText string

func (t *wireText) UnmarshalJSON(b []byte) error {
	var s string
	if json.Unmarshal(b, &s) == nil {
		*t = wireText(s)
		return nil
	}
	*t = wireText(joinTextChunks(b))
	return nil
}

func joinTextChunks(b []byte) string {
	var chunks []struct {
		Text string `json:"text"`
	}
	_ = json.Unmarshal(b, &chunks)
	var out strings.Builder
	for _, c := range chunks {
		out.WriteString(c.Text)
	}
	return out.String()
}
