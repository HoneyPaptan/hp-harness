package openai

import (
	"net/http"
	"time"

	"github.com/HoneyPaptan/hp-harness/internal/provider"
	"github.com/HoneyPaptan/hp-harness/internal/provider/transport"
)

var now = time.Now

func errorFromResponse(resp *http.Response) error {
	body := transport.ReadBody(resp.Body)
	delay := retryAfterFromHeaders(resp.Header)
	switch resp.StatusCode {
	case http.StatusTooManyRequests:
		return &provider.RateLimitError{RetryAfter: delay, Body: body}
	case 503, 529:
		return &provider.RateLimitError{Overloaded: true, RetryAfter: delay, Body: body}
	case 408, 409, 500, 502, 504:
		return &provider.TransientError{Err: &provider.HTTPError{Status: resp.StatusCode, Body: body}}
	default:
		return &provider.HTTPError{Status: resp.StatusCode, Body: body}
	}
}

func retryAfterFromHeaders(h http.Header) time.Duration {
	if v := h.Get("Retry-After"); v != "" {
		if d, ok := transport.RetryAfter(v, now()); ok {
			return d
		}
	}
	return maxResetDuration(h)
}

func maxResetDuration(h http.Header) time.Duration {
	var max time.Duration
	for _, key := range []string{"x-ratelimit-reset-requests", "x-ratelimit-reset-tokens"} {
		v := h.Get(key)
		if v == "" {
			continue
		}
		d, err := time.ParseDuration(v)
		if err != nil {
			continue
		}
		if d > max {
			max = d
		}
	}
	return max
}
