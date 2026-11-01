package anthropic

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"iter"
	"net/http"
	"strings"

	"github.com/HoneyPaptan/hp-harness/internal/provider"
	"github.com/HoneyPaptan/hp-harness/internal/provider/transport"
)

const apiVersion = "2023-06-01"

type Client struct {
	BaseURL   string
	APIKey    string
	MaxTokens int
	HTTP      *http.Client
}

func (c *Client) Stream(ctx context.Context, req provider.Request) iter.Seq2[provider.Event, error] {
	return func(yield func(provider.Event, error) bool) {
		c.runStream(ctx, req, yield)
	}
}

func (c *Client) runStream(ctx context.Context, req provider.Request, yield func(provider.Event, error) bool) {
	httpReq, err := c.newHTTPRequest(ctx, req)
	if err != nil {
		yield(provider.Event{}, err)
		return
	}
	resp, err := c.httpClient().Do(httpReq)
	if err != nil {
		yield(provider.Event{}, transport.Failure(ctx, err))
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		yield(provider.Event{}, errorFromResponse(resp))
		return
	}
	for ev, err := range streamEvents(eventSeq(ctx, resp.Body)) {
		if !yield(ev, err) || err != nil {
			return
		}
	}
}

func (c *Client) newHTTPRequest(ctx context.Context, req provider.Request) (*http.Request, error) {
	body, err := json.Marshal(buildRequest(req, c.MaxTokens))
	if err != nil {
		return nil, err
	}
	url := strings.TrimSuffix(c.BaseURL, "/") + "/messages"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	httpReq.Header.Set("anthropic-version", apiVersion)
	httpReq.Header.Set("x-api-key", c.APIKey)
	return httpReq, nil
}

func (c *Client) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return http.DefaultClient
}

func eventSeq(ctx context.Context, body io.Reader) iter.Seq2[wireEvent, error] {
	return func(yield func(wireEvent, error) bool) {
		for data, err := range transport.SSEData(body) {
			if err != nil {
				yield(wireEvent{}, transport.ReadFailure(ctx, err))
				return
			}
			var ev wireEvent
			if err := json.Unmarshal([]byte(data), &ev); err != nil {
				yield(wireEvent{}, transport.DecodeFailure(ctx, data, err))
				return
			}
			if ev.Error != nil {
				yield(wireEvent{}, ev.Error.asError())
				return
			}
			if !yield(ev, nil) {
				return
			}
		}
	}
}
