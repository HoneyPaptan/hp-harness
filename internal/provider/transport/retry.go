package transport

import (
	"io"
	"net/http"
	"strconv"
	"time"
)

const errBodyLimit = 4096

func ReadBody(r io.Reader) string {
	b, _ := io.ReadAll(io.LimitReader(r, errBodyLimit))
	return string(b)
}

func RetryAfter(v string, now time.Time) (time.Duration, bool) {
	sec, err := strconv.Atoi(v)
	if err == nil && sec >= 0 {
		return time.Duration(sec) * time.Second, true
	}
	t, err := http.ParseTime(v)
	if err != nil {
		return 0, false
	}
	return max(t.Sub(now), 0), true
}
