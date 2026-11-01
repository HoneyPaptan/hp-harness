package web

import (
	"context"
	"errors"
	"time"

	"github.com/HoneyPaptan/hp-harness/internal/core"
	"github.com/HoneyPaptan/hp-harness/internal/journal"
)

const (
	StatusRunning     = "running"
	StatusFinished    = "finished"
	StatusInterrupted = "interrupted"
)

var (
	ErrInvalid = errors.New("invalid request")
	ErrBusy    = errors.New("run is live in another process")
)

type State struct {
	Model   string   `json:"model"`
	Presets []string `json:"presets"`
	Jev     bool     `json:"jev"`
	Dir     string   `json:"dir"`
}

type Run struct {
	ID      string    `json:"id"`
	Title   string    `json:"title"`
	Model   string    `json:"model"`
	Status  string    `json:"status"`
	Updated time.Time `json:"updated"`
}

type JevSpend struct {
	Requests     int `json:"requests"`
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type Stats struct {
	Reason       string
	Steps        int
	InputTokens  int
	OutputTokens int
	TTFT         time.Duration
	Jev          *JevSpend
}

type Backend interface {
	State() State
	Runs() ([]Run, error)
	Records(id string) ([]journal.Record, error)
	Busy(id string) bool
	Start(model, prompt string) (Agent, error)
	Open(id string) (Agent, error)
}

type Agent interface {
	ID() string
	Model() string
	SetModel(spec string) error
	Send(ctx context.Context, prompt string, obs core.Observer) (Stats, error)
	Resume(ctx context.Context, obs core.Observer) (Stats, error)
	Release()
}
