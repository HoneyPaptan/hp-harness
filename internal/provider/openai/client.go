package openai

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

type Client struct {
	BaseURL         string
	APIKey          string
	HTTP            *http.Client
	DisableThinking bool
	NoStreamOptions bool
}

func (c *Client) Stream(ctx context.Context, req provider.Request) iter.Seq2[provider.Event, error] {
	return func(yield func(provider.Event, error) bool) {
		c.runStream(ctx, req, yield)
	}
}

func (c *Client) runStream(ctx context.Context, req provider.Request, yield func(provider.Event, error) bool) {
	body, err := c.encodeStreamRequest(req)
	if err != nil {
		yield(provider.Event{}, err)
		return
	}
	httpReq, err := c.newStreamHTTPRequest(ctx, body)
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
	for ev, err := range c.readStream(ctx, resp.Body) {
		if !yield(ev, err) {
			return
		}
		if err != nil {
			return
		}
	}
}

func (c *Client) encodeStreamRequest(req provider.Request) ([]byte, error) {
	w := buildRequest(req)
	w.Stream = true
	if !c.NoStreamOptions {
		w.StreamOptions = &wireStreamOptions{IncludeUsage: true}
	}
	if c.DisableThinking {
		w.Thinking = &wireThinking{Type: "disabled"}
	}
	return json.Marshal(w)
}

func (c *Client) newStreamHTTPRequest(ctx context.Context, body []byte) (*http.Request, error) {
	url := strings.TrimSuffix(c.BaseURL, "/") + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	if c.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.APIKey)
	}
	return httpReq, nil
}

func (c *Client) Preconnect(ctx context.Context) {
	transport.Preconnect(ctx, c.httpClient(), c.BaseURL)
}

func (c *Client) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return http.DefaultClient
}

func (c *Client) readStream(ctx context.Context, body io.Reader) iter.Seq2[provider.Event, error] {
	chunks := chunkSeq(ctx, body)
	return streamEvents(chunks)
}

func chunkSeq(ctx context.Context, body io.Reader) iter.Seq2[wireStreamChunk, error] {
	return func(yield func(wireStreamChunk, error) bool) {
		for data, err := range transport.SSEData(body) {
			if err != nil {
				yield(wireStreamChunk{}, transport.ReadFailure(ctx, err))
				return
			}
			chunk, err := decodeChunk(data)
			if err != nil {
				yield(wireStreamChunk{}, transport.DecodeFailure(ctx, data, err))
				return
			}
			if chunk.Error != nil {
				yield(wireStreamChunk{}, chunk.Error.asError())
				return
			}
			if !yield(chunk, nil) {
				return
			}
		}
	}
}
