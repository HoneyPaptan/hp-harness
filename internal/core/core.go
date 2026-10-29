package core

import (
	"context"
	"errors"

	"github.com/HoneyPaptan/hp-harness/internal/host"
	"github.com/HoneyPaptan/hp-harness/internal/journal"
	"github.com/HoneyPaptan/hp-harness/internal/provider"
	"github.com/HoneyPaptan/hp-harness/internal/tool"
)

const (
	ReasonFinal     = "final"
	ReasonStepLimit = "step_limit"
	defaultStepLimit = 50
)

var ErrFinished = errors.New("run already finished")

type Deps struct {
	Host     host.Host
	Provider provider.Provider
	Tools    *tool.Registry
	Observer Observer
}

type Config struct {
	RunID, Model, System, Dir, JournalPath, Prompt string
	StepLimit, MaxTokens                            int
}

type Outcome struct {
	Text   string
	Reason string
	Steps  int
	Usage  provider.Usage
}

func Run(ctx context.Context, d Deps, c Config) (Outcome, error) {
	start := runStartFromConfig(c)
	w, err := journal.Create(d.Host, c.JournalPath, start)
	if err != nil {
		return Outcome{}, err
	}
	if err := w.User(c.Prompt); err != nil {
		return Outcome{}, err
	}
	msgs := []provider.Message{{Role: provider.RoleUser, Text: c.Prompt}}
	r := newRunner(d, w, c, msgs, stepLimit(c))
	return r.run(ctx)
}

func runStartFromConfig(c Config) journal.RunStart {
	return journal.RunStart{
		RunID: c.RunID, Model: c.Model, System: c.System,
		Dir: c.Dir, StepLimit: stepLimit(c),
	}
}

func stepLimit(c Config) int {
	if c.StepLimit == 0 {
		return defaultStepLimit
	}
	return c.StepLimit
}
