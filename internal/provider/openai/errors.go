package openai

import (
	"net/http"
	"strconv"
	"strings"
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
		return &provider.TransientError{Err: httpError(resp.StatusCode, body)}
	default:
		return httpError(resp.StatusCode, body)
	}
}

func retryAfterFromHeaders(h http.Header) time.Duration {
	if v := h.Get("Retry-After"); v != "" {
		if d, ok := transport.RetryAfter(v, now()); ok {
			return d
		}
	}
	if d := resetAt(h.Get("x-ratelimit-reset")); d > 0 {
		return d
	}
	return maxResetDuration(h)
}

func resetAt(v string) time.Duration {
	n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
	if err != nil || n <= 0 {
		return 0
	}
	switch {
	case n > 1e12:
		return time.UnixMilli(n).Sub(now())
	case n > 1e9:
		return time.Unix(n, 0).Sub(now())
	default:
		return time.Duration(n) * time.Second
	}
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
