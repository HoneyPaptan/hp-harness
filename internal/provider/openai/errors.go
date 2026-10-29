package openai

import (
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/HoneyPaptan/hp-harness/internal/provider"
)

const errBodyLimit = 4096

func errorFromResponse(resp *http.Response) error {
	body := readLimited(resp.Body, errBodyLimit)
	if resp.StatusCode == http.StatusTooManyRequests {
		return &provider.RateLimitError{
			RetryAfter: parseRetryAfter(resp.Header.Get("Retry-After")),
			Body:       body,
		}
	}
	return &provider.HTTPError{Status: resp.StatusCode, Body: body}
}

func readLimited(r io.Reader, n int) string {
	b, _ := io.ReadAll(io.LimitReader(r, int64(n)))
	return string(b)
}

func parseRetryAfter(v string) time.Duration {
	if v == "" {
		return 0
	}
	sec, err := strconv.Atoi(v)
	if err != nil || sec < 0 {
		return 0
	}
	return time.Duration(sec) * time.Second
}
