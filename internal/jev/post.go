package jev

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

func (c *Client) askChunk(ctx context.Context, state any, qs map[string]Question) (Answers, Usage, error) {
	body, err := encodeRequest(c.Model, state, qs)
	if err != nil {
		return nil, Usage{}, err
	}
	respBody, err := c.post(ctx, body)
	if err != nil {
		return nil, Usage{}, err
	}
	return decodeResponse(respBody)
}

func (c *Client) post(ctx context.Context, body []byte) ([]byte, error) {
	backoff := []time.Duration{time.Second, 2 * time.Second}
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			if err := c.wait(ctx, backoff[attempt-1]); err != nil {
				return nil, err
			}
		}
		resp, err := c.doPost(ctx, body)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			respBody, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil {
				return nil, err
			}
			return respBody, nil
		}
		respBody := readLimited(resp.Body, errBodyLimit)
		resp.Body.Close()
		lastErr = fmt.Errorf("http %d: %s", resp.StatusCode, respBody)
		if resp.StatusCode == 429 || resp.StatusCode == 529 {
			if attempt < 2 {
				continue
			}
		}
		return nil, lastErr
	}
	return nil, lastErr
}

func (c *Client) doPost(ctx context.Context, body []byte) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.URL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.Key)
	return c.httpClient().Do(req)
}

func readLimited(r io.Reader, n int) string {
	b, _ := io.ReadAll(io.LimitReader(r, int64(n)))
	return string(b)
}
