package core

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/HoneyPaptan/hp-harness/internal/provider"
)

var ErrLimitTooLong = errors.New("rate limit wait exceeds max wait")

const (
	defaultSwitchAfter = 20 * time.Second
	defaultMaxWait     = 6 * time.Hour
	backoffStart       = 2 * time.Second
	backoffCap         = 60 * time.Second
)

func switchAfter(c Config) time.Duration {
	if c.SwitchAfter > 0 {
		return c.SwitchAfter
	}
	return defaultSwitchAfter
}

func maxWait(c Config) time.Duration {
	if c.MaxWait > 0 {
		return c.MaxWait
	}
	return defaultMaxWait
}

func modelSpec(c Config) string {
	return c.Preset + "/" + c.Model
}

func limitDelay(rl *provider.RateLimitError, n *int) time.Duration {
	if rl.RetryAfter > 0 {
		*n = 0
		return rl.RetryAfter
	}
	d := backoff(*n)
	*n++
	return d
}

func backoff(n int) time.Duration {
	d := backoffStart
	for i := 0; i < n; i++ {
		d *= 2
		if d > backoffCap {
			return backoffCap
		}
	}
	return d
}

func (r *runner) waitLimit(ctx context.Context, spec string, delay time.Duration) error {
	if delay > maxWait(r.cfg) {
		return fmt.Errorf("%w: %s", ErrLimitTooLong, delay)
	}
	now := r.deps.Host.Now()
	until := now.Add(delay)
	if err := r.w.LimitWait(spec, until); err != nil {
		return err
	}
	r.deps.Observer.emitWait(spec, delay, until)
	if err := r.deps.Host.Sleep(ctx, delay); err != nil {
		return err
	}
	return nil
}
