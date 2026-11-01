package transport

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestRetryAfter(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	cases := []struct {
		name  string
		value string
		want  time.Duration
		ok    bool
	}{
		{name: "seconds", value: "7", want: 7 * time.Second, ok: true},
		{name: "date", value: now.Add(time.Minute).Format(http.TimeFormat), want: time.Minute, ok: true},
		{name: "past date", value: now.Add(-time.Minute).Format(http.TimeFormat), ok: true},
		{name: "negative", value: "-1"},
		{name: "garbage", value: "soon"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := RetryAfter(tc.value, now)
			if got != tc.want || ok != tc.ok {
				t.Fatalf("got %v %v, want %v %v", got, ok, tc.want, tc.ok)
			}
		})
	}
}

func TestReadBodyLimit(t *testing.T) {
	if got := ReadBody(strings.NewReader(strings.Repeat("x", errBodyLimit+1))); len(got) != errBodyLimit {
		t.Fatalf("read %d bytes", len(got))
	}
}
