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
	Overloaded bool
	RetryAfter time.Duration
	Body       string
}

func (e *RateLimitError) Error() string {
	kind := "rate limited"
	if e.Overloaded {
		kind = "overloaded"
	}
	if e.RetryAfter > 0 {
		return fmt.Sprintf("http 429: %s, retry after %s: %s", kind, e.RetryAfter, e.Body)
	}
	return fmt.Sprintf("http 429: %s: %s", kind, e.Body)
}
