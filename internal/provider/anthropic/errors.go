package anthropic

import (
	"fmt"
	"net/http"
	"time"

	"github.com/HoneyPaptan/hp-harness/internal/provider"
	"github.com/HoneyPaptan/hp-harness/internal/provider/transport"
)

const statusOverloaded = 529

var (
	now = time.Now

	resetHeaders = []string{
		"anthropic-ratelimit-requests-reset",
		"anthropic-ratelimit-tokens-reset",
		"anthropic-ratelimit-input-tokens-reset",
		"anthropic-ratelimit-output-tokens-reset",
	}

	permanentKinds = map[string]int{
		"invalid_request_error": 400,
		"authentication_error":  401,
		"billing_error":         402,
		"permission_error":      403,
		"not_found_error":       404,
		"request_too_large":     413,
	}
)

func errorFromResponse(resp *http.Response) error {
	body := transport.ReadBody(resp.Body)
	switch status := resp.StatusCode; {
	case status == http.StatusTooManyRequests:
		return &provider.RateLimitError{RetryAfter: retryDelay(resp.Header), Body: body}
	case status == statusOverloaded:
		return &provider.RateLimitError{Overloaded: true, RetryAfter: retryDelay(resp.Header), Body: body}
	case status == 408 || status == 409 || status >= 500:
		return &provider.TransientError{Err: &provider.HTTPError{Status: status, Body: body}}
	default:
		return &provider.HTTPError{Status: status, Body: body}
	}
}

func retryDelay(h http.Header) time.Duration {
	if v := h.Get("Retry-After"); v != "" {
		if d, ok := transport.RetryAfter(v, now()); ok {
			return d
		}
	}
	return latestReset(h)
}

func latestReset(h http.Header) time.Duration {
	var latest time.Duration
	for _, key := range resetHeaders {
		t, err := time.Parse(time.RFC3339, h.Get(key))
		if err == nil {
			latest = max(latest, t.Sub(now()))
		}
	}
	return latest
}

func (e *wireEventError) asError() error {
	if status, ok := permanentKinds[e.Type]; ok {
		return &provider.HTTPError{Status: status, Body: e.Message}
	}
	switch e.Type {
	case "rate_limit_error":
		return &provider.RateLimitError{Body: e.Message}
	case "overloaded_error":
		return &provider.RateLimitError{Overloaded: true, Body: e.Message}
	}
	return &provider.TransientError{Err: fmt.Errorf("stream error: %s", e.Message)}
}
