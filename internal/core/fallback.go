package core

import (
	"context"
	"fmt"
	"time"

	"github.com/HoneyPaptan/hp-harness/internal/provider"
)

func (r *runner) allSpecs() []string {
	out := []string{modelSpec(r.cfg)}
	out = append(out, r.cfg.Fallbacks...)
	return out
}

func (r *runner) bindSpec(d Deps, spec string) error {
	r.spec = spec
	if d.Providers != nil {
		p, model, err := d.Providers(spec)
		if err != nil {
			return err
		}
		r.prov = p
		r.reqModel = model
		return nil
	}
	r.prov = d.Provider
	r.reqModel = r.cfg.Model
	return nil
}

func (r *runner) specFree(spec string, now time.Time) bool {
	until, ok := r.limited[spec]
	return !ok || !until.After(now)
}

func (r *runner) pickFreeFallback(now time.Time) string {
	for _, spec := range r.allSpecs() {
		if spec == r.spec {
			continue
		}
		if r.specFree(spec, now) {
			return spec
		}
	}
	return ""
}

func (r *runner) pickEarliestLimited() (string, time.Time) {
	var best string
	var until time.Time
	for _, spec := range r.allSpecs() {
		u, ok := r.limited[spec]
		if !ok {
			continue
		}
		if best == "" || u.Before(until) {
			best = spec
			until = u
		}
	}
	return best, until
}

func (r *runner) switchSpec(to string) error {
	from := r.spec
	if err := r.w.ModelSwitch(from, to); err != nil {
		return err
	}
	r.deps.Observer.emitSwitch(from, to)
	return r.bindSpec(r.deps, to)
}

func (r *runner) handleRateLimit(ctx context.Context, rl *provider.RateLimitError, n *int) error {
	delay := limitDelay(rl, n)
	now := r.deps.Host.Now()
	r.limited[r.spec] = now.Add(delay)
	if delay > switchAfter(r.cfg) && r.deps.Providers != nil {
		if next := r.pickFreeFallback(now); next != "" {
			return r.switchSpec(next)
		}
		waitSpec, until := r.pickEarliestLimited()
		wait := until.Sub(now)
		if wait > maxWait(r.cfg) {
			return fmt.Errorf("%w: %s", ErrLimitTooLong, wait)
		}
		if waitSpec != r.spec {
			if err := r.switchSpec(waitSpec); err != nil {
				return err
			}
		}
		return r.waitLimit(ctx, waitSpec, wait)
	}
	if delay > maxWait(r.cfg) {
		return fmt.Errorf("%w: %s", ErrLimitTooLong, delay)
	}
	return r.waitLimit(ctx, r.spec, delay)
}
