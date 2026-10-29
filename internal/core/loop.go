package core

import (
	"context"

	"github.com/HoneyPaptan/hp-harness/internal/journal"
	"github.com/HoneyPaptan/hp-harness/internal/provider"
)

type runner struct {
	deps  Deps
	w     *journal.Writer
	cfg   Config
	msgs  []provider.Message
	steps int
	limit int
	usage provider.Usage
}

func newRunner(d Deps, w *journal.Writer, c Config, msgs []provider.Message, limit int) *runner {
	return &runner{deps: d, w: w, cfg: c, msgs: msgs, limit: limit}
}

func (r *runner) run(ctx context.Context) (Outcome, error) {
	for {
		out, done, err := r.turn(ctx)
		if err != nil {
			return Outcome{}, err
		}
		if done {
			return out, nil
		}
	}
}

func (r *runner) turn(ctx context.Context) (Outcome, bool, error) {
	msg, err := r.collectTurn(ctx)
	if err != nil {
		return Outcome{}, false, err
	}
	if err := r.w.Assistant(msg); err != nil {
		return Outcome{}, false, err
	}
	r.msgs = append(r.msgs, msg)
	if len(msg.ToolCalls) == 0 {
		return r.finishFinal(msg.Text)
	}
	return r.afterTools(ctx, msg.ToolCalls)
}

func (r *runner) collectTurn(ctx context.Context) (provider.Message, error) {
	req := provider.Request{
		Model: r.cfg.Model, System: r.cfg.System, Messages: r.msgs,
		Tools: r.deps.Tools.Defs(), MaxTokens: r.cfg.MaxTokens,
	}
	raw := r.deps.Provider.Stream(ctx, req)
	seq := tap(raw, r.deps.Observer.emitEvent)
	msg, u, err := provider.Collect(seq)
	if err != nil {
		return provider.Message{}, err
	}
	r.usage.InputTokens += u.InputTokens
	r.usage.OutputTokens += u.OutputTokens
	return msg, nil
}

func (r *runner) afterTools(ctx context.Context, calls []provider.ToolCall) (Outcome, bool, error) {
	results, err := r.runTools(ctx, calls)
	if err != nil {
		return Outcome{}, false, err
	}
	r.msgs = appendResults(r.msgs, results)
	r.steps++
	if r.steps >= r.limit {
		return r.finishLimit()
	}
	return Outcome{}, false, nil
}

func (r *runner) finishFinal(text string) (Outcome, bool, error) {
	if err := r.w.End(ReasonFinal); err != nil {
		return Outcome{}, false, err
	}
	o := Outcome{Text: text, Reason: ReasonFinal, Steps: r.steps, Usage: r.usage}
	return o, true, nil
}

func (r *runner) finishLimit() (Outcome, bool, error) {
	if err := r.w.End(ReasonStepLimit); err != nil {
		return Outcome{}, false, err
	}
	o := Outcome{Reason: ReasonStepLimit, Steps: r.steps, Usage: r.usage}
	return o, true, nil
}
