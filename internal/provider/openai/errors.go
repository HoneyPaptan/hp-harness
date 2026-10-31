package openai

import (
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/HoneyPaptan/hp-harness/internal/provider"
)

const errBodyLimit = 4096

var now = time.Now

func errorFromResponse(resp *http.Response) error {
	body := readLimited(resp.Body, errBodyLimit)
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

func readLimited(r io.Reader, n int) string {
	b, _ := io.ReadAll(io.LimitReader(r, int64(n)))
	return string(b)
}

func retryAfterFromHeaders(h http.Header) time.Duration {
	if v := h.Get("Retry-After"); v != "" {
		if d, ok := parseRetryAfterValue(v); ok {
			return d
		}
	}
	return maxResetDuration(h)
}

func parseRetryAfterValue(v string) (time.Duration, bool) {
	sec, err := strconv.Atoi(v)
	if err == nil && sec >= 0 {
		return time.Duration(sec) * time.Second, true
	}
	t, err := http.ParseTime(v)
	if err != nil {
		return 0, false
	}
	d := t.Sub(now())
	if d < 0 {
		return 0, true
	}
	return d, true
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
