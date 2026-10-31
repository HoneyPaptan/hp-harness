package core

import (
	"context"
	"errors"
	"time"

	"github.com/HoneyPaptan/hp-harness/internal/journal"
	"github.com/HoneyPaptan/hp-harness/internal/provider"
)

type runner struct {
	deps     Deps
	w        *journal.Writer
	cfg      Config
	msgs     []provider.Message
	steps    int
	limit    int
	usage    provider.Usage
	spec     string
	reqModel string
	prov     provider.Provider
	limited  map[string]time.Time
}

func newRunner(d Deps, w *journal.Writer, c Config, msgs []provider.Message, limit int) (*runner, error) {
	r := &runner{
		deps: d, w: w, cfg: c, msgs: msgs, limit: limit,
		limited: make(map[string]time.Time),
	}
	if err := r.bindSpec(d, modelSpec(c)); err != nil {
		return nil, err
	}
	return r, nil
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
	limitN, failures := 0, 0
	tools := r.turnTools(ctx)
	for {
		msg, u, err := r.streamTurn(ctx, tools)
		if err == nil {
			r.usage.InputTokens += u.InputTokens
			r.usage.OutputTokens += u.OutputTokens
			return msg, nil
		}
		if err := r.recoverTurn(ctx, err, &limitN, &failures); err != nil {
			return provider.Message{}, err
		}
	}
}

func (r *runner) recoverTurn(ctx context.Context, err error, limitN, failures *int) error {
	var rl *provider.RateLimitError
	if errors.As(err, &rl) {
		return r.handleRateLimit(ctx, rl, limitN)
	}
	var te *provider.TransientError
	if errors.As(err, &te) {
		return r.retryStream(ctx, err, failures)
	}
	return err
}

func (r *runner) streamTurn(ctx context.Context, tools []provider.ToolDef) (provider.Message, provider.Usage, error) {
	req := provider.Request{
		Model: r.reqModel, System: r.cfg.System, Messages: r.msgs,
		Tools: tools, MaxTokens: r.cfg.MaxTokens,
	}
	r.deps.Observer.emitTurnStart()
	raw := r.prov.Stream(ctx, req)
	seq := tap(raw, r.deps.Observer.emitEvent)
	return provider.Collect(seq)
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
