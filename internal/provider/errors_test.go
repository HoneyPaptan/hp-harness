package provider

import (
	"errors"
	"testing"
	"time"
)

func TestHTTPError(t *testing.T) {
	err := &HTTPError{Status: 400, Body: `{"error":"bad"}`}
	if got := err.Error(); got != `http 400: {"error":"bad"}` {
		t.Fatalf("Error() = %q", got)
	}
	var he *HTTPError
	if !errors.As(err, &he) || he.Status != 400 {
		t.Fatal("errors.As HTTPError")
	}
}

func TestErrContextOverflowIs(t *testing.T) {
	he := &HTTPError{Status: 400, Body: "long", Overflow: true}
	if !errors.Is(he, ErrContextOverflow) {
		t.Fatal("direct overflow")
	}
	if errors.Is(&HTTPError{Status: 400, Overflow: false}, ErrContextOverflow) {
		t.Fatal("non-overflow")
	}
	wrapped := &TransientError{Err: he}
	if !errors.Is(wrapped, ErrContextOverflow) {
		t.Fatal("wrapped overflow")
	}
}

func TestTransientError(t *testing.T) {
	cause := errors.New("reset")
	err := error(&TransientError{Err: cause})
	if got := err.Error(); got != "reset" {
		t.Fatalf("Error() = %q", got)
	}
	if !errors.Is(err, cause) {
		t.Fatal("errors.Is cause")
	}
}

func TestRateLimitError(t *testing.T) {
	withDelay := &RateLimitError{RetryAfter: 30 * time.Second, Body: "slow"}
	if got := withDelay.Error(); got != "http 429: rate limited, retry after 30s: slow" {
		t.Fatalf("Error() = %q", got)
	}
	noDelay := &RateLimitError{Body: "slow"}
	if got := noDelay.Error(); got != "http 429: rate limited: slow" {
		t.Fatalf("Error() = %q", got)
	}
	var rl *RateLimitError
	if !errors.As(withDelay, &rl) || rl.RetryAfter != 30*time.Second {
		t.Fatal("errors.As RateLimitError")
	}
}
