package provider

import (
	"context"
	"encoding/json"
	"errors"
	"iter"
)

type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

type ToolCall struct {
	ID    string
	Name  string
	Args  json.RawMessage
	Extra json.RawMessage `json:",omitempty"`
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
	Effort    string
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
	EventReasoning
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

type Preconnector interface {
	Preconnect(ctx context.Context)
}

func Preconnect(ctx context.Context, p Provider) {
	if c, ok := p.(Preconnector); ok {
		c.Preconnect(ctx)
	}
}

type ModelLister interface {
	ListModels(ctx context.Context) ([]string, error)
}

var ErrNoModelList = errors.New("this provider cannot list models")

func ListModels(ctx context.Context, p Provider) ([]string, error) {
	if l, ok := p.(ModelLister); ok {
		return l.ListModels(ctx)
	}
	return nil, ErrNoModelList
}
