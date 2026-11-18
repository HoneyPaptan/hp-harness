package tui

import (
	"context"
	"errors"
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
	EventApprove
)

type ApproveAnswer int

const (
	ApproveYes ApproveAnswer = iota
	ApproveAlways
	ApproveNo
)

type Event struct {
	Kind  EventKind
	Text  string
	Reply chan<- ApproveAnswer
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
	Models(ctx context.Context, preset string) ([]string, error)
	Providers() []string
	Jev() bool
	SetModel(spec string) error
	SetJev(on bool) error
	SaveKey(name, value string) error
	Reset() error
	Runs() ([]string, error)
	Files() ([]string, error)
	PermissionMode() string
	SetPermissionMode(string) error
	PermissionInfo() string
	Effort() string
	SetEffort(string) error
	Compact(ctx context.Context) (bool, error)
}

type KeyMissingError struct{ Hint string }

func (e KeyMissingError) Error() string { return e.Hint }

type NoModelError struct{}

func (NoModelError) Error() string { return "pick a model first" }

func IsNoModel(err error) bool {
	var target NoModelError
	return errors.As(err, &target)
}

type KeyHinter interface {
	KeyHint() string
}

type QuitNoter interface {
	QuitNotes() []string
}

type Preconnector interface {
	Preconnect(ctx context.Context)
}
