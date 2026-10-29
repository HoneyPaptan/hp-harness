package core

import (
	"context"

	"github.com/HoneyPaptan/hp-harness/internal/host"
	"github.com/HoneyPaptan/hp-harness/internal/journal"
	"github.com/HoneyPaptan/hp-harness/internal/provider"
	"github.com/HoneyPaptan/hp-harness/internal/tool"
)

const interruptedMsg = "interrupted: the previous process stopped while this tool was running; its outcome is unknown"

func Resume(ctx context.Context, d Deps, journalPath string) (Outcome, error) {
	w, st, err := openJournal(d.Host, journalPath)
	if err != nil {
		return Outcome{}, err
	}
	if st.Finished {
		return Outcome{}, ErrFinished
	}
	r := newResumeRunner(d, w, journalPath, st)
	if err := r.settlePending(ctx, st.Pending); err != nil {
		return Outcome{}, err
	}
	if out, ok, err := r.maybeFinishAfterCrash(); ok {
		return out, err
	}
	return r.run(ctx)
}

func openJournal(h host.Host, path string) (*journal.Writer, journal.State, error) {
	w, recs, err := journal.Open(h, path)
	if err != nil {
		return nil, journal.State{}, err
	}
	st, err := journal.Replay(recs)
	if err != nil {
		return nil, journal.State{}, err
	}
	return w, st, nil
}

func newResumeRunner(d Deps, w *journal.Writer, path string, st journal.State) *runner {
	cfg := Config{
		RunID: st.Start.RunID, Model: st.Start.Model, System: st.Start.System,
		Dir: st.Start.Dir, JournalPath: path, StepLimit: st.Start.StepLimit,
		Preset: st.Start.Preset, BaseURL: st.Start.BaseURL, KeyEnv: st.Start.KeyEnv,
		MaxTokens: st.Start.MaxTokens,
	}
	return &runner{
		deps: d, w: w, cfg: cfg,
		msgs:  append([]provider.Message(nil), st.Messages...),
		steps: st.Steps, limit: st.Start.StepLimit,
	}
}

func (r *runner) settlePending(ctx context.Context, pending []journal.Pending) error {
	if len(pending) == 0 {
		return nil
	}
	env := tool.Env{Host: r.deps.Host, Dir: r.cfg.Dir}
	out := make([]provider.ToolResult, 0, len(pending))
	for _, p := range pending {
		tr, err := r.settleOne(ctx, env, p)
		if err != nil {
			return err
		}
		out = append(out, tr)
	}
	r.msgs = appendResults(r.msgs, out)
	return nil
}

func (r *runner) settleOne(ctx context.Context, env tool.Env, p journal.Pending) (provider.ToolResult, error) {
	if p.Started {
		tr := provider.ToolResult{CallID: p.Call.ID, Text: interruptedMsg, IsError: true}
		if err := r.w.ToolResult(tr); err != nil {
			return provider.ToolResult{}, err
		}
		r.deps.Observer.emitToolResult(tr)
		return tr, nil
	}
	return r.execCall(ctx, env, p.Call)
}

func (r *runner) maybeFinishAfterCrash() (Outcome, bool, error) {
	n := len(r.msgs)
	if n == 0 {
		return Outcome{}, false, nil
	}
	last := r.msgs[n-1]
	if last.Role != provider.RoleAssistant || len(last.ToolCalls) != 0 {
		return Outcome{}, false, nil
	}
	return r.finishFinal(last.Text)
}
