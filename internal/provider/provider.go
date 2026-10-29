package provider

import (
	"context"
	"encoding/json"
	"iter"
)

type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

type ToolCall struct {
	ID   string
	Name string
	Args json.RawMessage
}

type ToolResult struct {
	CallID  string
	Text    string
	IsError bool
}

type Message struct {
	Role        Role
	Text        string
	ToolCalls   []ToolCall
	ToolResults []ToolResult
}

type ToolDef struct {
	Name        string
	Description string
	Schema      json.RawMessage
}

type Request struct {
	Model     string
	System    string
	Messages  []Message
	Tools     []ToolDef
	MaxTokens int
}

type Usage struct {
	InputTokens  int
	OutputTokens int
}

type EventKind int

const (
	EventText EventKind = iota
	EventToolCall
	EventDone
)

type Event struct {
	Kind       EventKind
	Text       string
	Call       ToolCall
	Usage      Usage
	StopReason string
}

type Provider interface {
	Stream(ctx context.Context, req Request) iter.Seq2[Event, error]
}
