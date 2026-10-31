package jev

import (
	"context"
	"net/http"
	"sync"
	"time"
)

type Client struct {
	URL   string
	Key   string
	Model string
	HTTP  *http.Client
	Sleep func(time.Duration)

	mu    sync.Mutex
	spent Spend
}

func (c *Client) Ask(ctx context.Context, state any, qs map[string]Question) (Answers, Usage, error) {
	chunks := splitQuestions(qs)
	if len(chunks) == 1 {
		return c.askChunk(ctx, state, chunks[0])
	}
	return c.askChunks(ctx, state, chunks)
}

func (c *Client) askChunks(ctx context.Context, state any, chunks []map[string]Question) (Answers, Usage, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	type result struct {
		answers Answers
		usage   Usage
	}

	sem := make(chan struct{}, maxConcurrentAsks)
	results := make([]result, len(chunks))
	var wg sync.WaitGroup
	var once sync.Once
	var firstErr error
	fail := func(err error) {
		once.Do(func() {
			firstErr = err
			cancel()
		})
	}

	for i, chunk := range chunks {
		wg.Add(1)
		go func(i int, chunk map[string]Question) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				fail(ctx.Err())
				return
			}
			if err := ctx.Err(); err != nil {
				fail(err)
				return
			}
			ans, use, err := c.askChunk(ctx, state, chunk)
			if err != nil {
				fail(err)
				return
			}
			results[i] = result{answers: ans, usage: use}
		}(i, chunk)
	}
	wg.Wait()

	if firstErr != nil {
		return nil, Usage{}, firstErr
	}
	merged := make(Answers)
	var total Usage
	for _, r := range results {
		for k, v := range r.answers {
			merged[k] = v
		}
		total = total.add(r.usage)
	}
	return merged, total, nil
}

func (c *Client) wait(ctx context.Context, d time.Duration) error {
	if c.Sleep != nil {
		c.Sleep(d)
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *Client) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return http.DefaultClient
}
