package core

import (
	"context"
	"errors"

	"github.com/HoneyPaptan/hp-harness/internal/journal"
	"github.com/HoneyPaptan/hp-harness/internal/provider"
)

const (
	stoppedToolMsg = "interrupted: the user stopped this tool while it was running; its outcome is unknown"
	skippedToolMsg = "interrupted: the user stopped the turn before this tool ran; it did not run"
)

var ErrUnfinished = errors.New("run not finished: resume it before continuing")

func Continue(ctx context.Context, d Deps, journalPath, prompt string) (Outcome, error) {
	return continueRun(ctx, d, journalPath, prompt, false)
}

func ContinueInterrupted(ctx context.Context, d Deps, journalPath, prompt string) (Outcome, error) {
	return continueRun(ctx, d, journalPath, prompt, true)
}

func continueRun(ctx context.Context, d Deps, journalPath, prompt string, allowOpen bool) (Outcome, error) {
	w, st, err := openJournal(d.Host, journalPath)
	if err != nil {
		return Outcome{}, err
	}
	if !st.Finished && !allowOpen {
		return Outcome{}, ErrUnfinished
	}
	r, err := newResumeRunner(d, w, journalPath, st)
	if err != nil {
		return Outcome{}, err
	}
	if err := r.abandonPending(st.Pending); err != nil {
		return Outcome{}, err
	}
	if err := r.addPrompt(prompt); err != nil {
		return Outcome{}, err
	}
	return r.run(ctx)
}

func (r *runner) abandonPending(pending []journal.Pending) error {
	out := make([]provider.ToolResult, 0, len(pending))
	for _, p := range pending {
		tr, err := r.abandonCall(p.Call.ID, abandonText(p))
		if err != nil {
			return err
		}
		out = append(out, tr)
	}
	r.msgs = appendResults(r.msgs, out)
	return nil
}

func abandonText(p journal.Pending) string {
	if p.Started {
		return stoppedToolMsg
	}
	return skippedToolMsg
}

func (r *runner) addPrompt(prompt string) error {
	if err := r.w.User(prompt); err != nil {
		return err
	}
	r.msgs = append(r.msgs, provider.Message{Role: provider.RoleUser, Text: prompt})
	r.steps = 0
	return nil
}
