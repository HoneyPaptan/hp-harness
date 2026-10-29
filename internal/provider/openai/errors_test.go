package openai

import (
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/HoneyPaptan/hp-harness/internal/provider"
)

func TestRetryAfterFromHeaders(t *testing.T) {
	fixed := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	oldNow := now
	now = func() time.Time { return fixed }
	t.Cleanup(func() { now = oldNow })

	tests := []struct {
		name   string
		header http.Header
		want   time.Duration
	}{
		{
			name:   "seconds",
			header: http.Header{"Retry-After": []string{"30"}},
			want:   30 * time.Second,
		},
		{
			name: "http date",
			header: http.Header{
				"Retry-After": []string{fixed.Add(45 * time.Second).UTC().Format(http.TimeFormat)},
			},
			want: 45 * time.Second,
		},
		{
			name: "groq headers max",
			header: http.Header{
				"X-Ratelimit-Reset-Requests": []string{"2.5s"},
				"X-Ratelimit-Reset-Tokens":   []string{"1m4s"},
			},
			want: 64 * time.Second,
		},
		{
			name: "bad header zero",
			header: http.Header{
				"Retry-After":                []string{"nope"},
				"X-Ratelimit-Reset-Requests": []string{"bad"},
			},
			want: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := retryAfterFromHeaders(tc.header); got != tc.want {
				t.Fatalf("retryAfterFromHeaders() = %v want %v", got, tc.want)
			}
		})
	}
}

func TestErrorFromResponseStatus(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		header     http.Header
		wantRL     bool
		overloaded bool
		wantHTTP   bool
	}{
		{name: "529 overloaded", status: 529, wantRL: true, overloaded: true},
		{name: "503 overloaded", status: 503, wantRL: true, overloaded: true},
		{name: "500 http error", status: 500, wantHTTP: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resp := &http.Response{
				StatusCode: tc.status,
				Header:     tc.header,
				Body:       http.NoBody,
			}
			err := errorFromResponse(resp)
			var rl *provider.RateLimitError
			var he *provider.HTTPError
			switch {
			case tc.wantRL:
				if !errors.As(err, &rl) {
					t.Fatalf("err = %v", err)
				}
				if rl.Overloaded != tc.overloaded {
					t.Fatalf("Overloaded = %v", rl.Overloaded)
				}
			case tc.wantHTTP:
				if !errors.As(err, &he) || he.Status != tc.status {
					t.Fatalf("err = %v", err)
				}
			}
		})
	}
}

func TestErrorFromResponseOverloadedMessage(t *testing.T) {
	resp := &http.Response{StatusCode: 529, Header: http.Header{}, Body: http.NoBody}
	err := errorFromResponse(resp)
	if !strings.Contains(err.Error(), "overloaded") {
		t.Fatalf("Error() = %q", err.Error())
	}
}
