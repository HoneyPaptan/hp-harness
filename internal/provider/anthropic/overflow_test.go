package anthropic

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/HoneyPaptan/hp-harness/internal/provider"
)

func TestErrorFromResponseOverflow(t *testing.T) {
	body := `{"type":"error","error":{"type":"invalid_request_error","message":"prompt is too long: 210000 tokens > 200000 maximum"}}`
	resp := &http.Response{
		StatusCode: 400,
		Body:       io.NopCloser(strings.NewReader(body)),
	}
	err := errorFromResponse(resp)
	var he *provider.HTTPError
	if !errors.As(err, &he) || !errors.Is(err, provider.ErrContextOverflow) {
		t.Fatalf("err = %#v", err)
	}
}

func TestErrorFromResponse429NotOverflow(t *testing.T) {
	resp := &http.Response{
		StatusCode: 429,
		Body:       io.NopCloser(strings.NewReader(`{"type":"error","error":{"type":"rate_limit_error","message":"slow"}}`)),
	}
	err := errorFromResponse(resp)
	var rl *provider.RateLimitError
	var he *provider.HTTPError
	if !errors.As(err, &rl) || errors.As(err, &he) || errors.Is(err, provider.ErrContextOverflow) {
		t.Fatalf("err = %#v", err)
	}
}
