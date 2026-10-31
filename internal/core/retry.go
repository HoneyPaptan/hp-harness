package core

import "context"

const maxStreamAttempts = 5

func (r *runner) retryStream(ctx context.Context, err error, failures *int) error {
	if cause := ctx.Err(); cause != nil {
		return cause
	}
	*failures++
	if *failures >= maxStreamAttempts {
		return err
	}
	delay := backoff(*failures - 1)
	r.deps.Observer.emitRetry(delay, *failures, maxStreamAttempts)
	return r.deps.Host.Sleep(ctx, delay)
}
