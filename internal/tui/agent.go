package tui

import (
	"context"
	"time"
)

type EventKind int

const (
	EventStep EventKind = iota
	EventText
	EventTool
	EventToolError
	EventNote
	EventThinking
)

type Event struct {
	Kind EventKind
	Text string
}

type Outcome struct {
	Model                     string
	Steps                     int
	InputTokens, OutputTokens int
	TTFT                      time.Duration
	Jev, Note                 string
}

type Agent interface {
	Send(ctx context.Context, prompt string, emit func(Event)) (Outcome, error)
	Resume(ctx context.Context, emit func(Event)) (Outcome, error)
	Model() string
	SetModel(spec string) error
	Reset() error
	Runs() ([]string, error)
}

type Preconnector interface {
	Preconnect(ctx context.Context)
}
