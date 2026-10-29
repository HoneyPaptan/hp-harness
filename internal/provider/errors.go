package provider

import (
	"fmt"
	"time"
)

type HTTPError struct {
	Status int
	Body   string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("http %d: %s", e.Status, e.Body)
}

type RateLimitError struct {
	RetryAfter time.Duration
	Body       string
}

func (e *RateLimitError) Error() string {
	if e.RetryAfter > 0 {
		return fmt.Sprintf("http 429: rate limited, retry after %s: %s", e.RetryAfter, e.Body)
	}
	return fmt.Sprintf("http 429: rate limited: %s", e.Body)
}
