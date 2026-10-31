package openai

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/HoneyPaptan/hp-harness/internal/provider"
)

func readAll(ctx context.Context, body io.Reader) error {
	_, err := collectAll((&Client{}).readStream(ctx, body))
	return err
}

func TestStreamErrorFrame(t *testing.T) {
	tests := []struct {
		name       string
		frame      string
		wantRL     bool
		overloaded bool
		wantStatus int
		wantText   string
	}{
		{name: "rate limit code", frame: `{"error":{"message":"slow","code":"rate_limit_exceeded"}}`, wantRL: true, wantText: "slow"},
		{name: "rate limit type", frame: `{"error":{"message":"slow","type":"rate_limit_exceeded"}}`, wantRL: true, wantText: "slow"},
		{name: "numeric 429", frame: `{"error":{"message":"slow","code":429}}`, wantRL: true, wantText: "slow"},
		{name: "string 429", frame: `{"error":{"message":"slow","code":"429"}}`, wantRL: true, wantText: "slow"},
		{name: "overloaded type", frame: `{"error":{"message":"busy","type":"overloaded"}}`, wantRL: true, overloaded: true, wantText: "busy"},
		{name: "numeric 503", frame: `{"error":{"message":"busy","code":503}}`, wantRL: true, overloaded: true, wantText: "busy"},
		{name: "numeric 529", frame: `{"error":{"message":"busy","code":529}}`, wantRL: true, overloaded: true, wantText: "busy"},
		{name: "server error", frame: `{"error":{"message":"boom","type":"internal_server_error","code":null}}`, wantText: "boom"},
		{name: "numeric 500", frame: `{"error":{"message":"boom","code":500}}`, wantText: "boom"},
		{name: "bare string", frame: `{"error":"boom"}`, wantText: "boom"},
		{name: "numeric 408", frame: `{"error":{"message":"boom","code":408}}`, wantText: "boom"},
		{name: "numeric 409", frame: `{"error":{"message":"boom","code":409}}`, wantText: "boom"},
		{name: "numeric 400", frame: `{"error":{"message":"no","code":400}}`, wantStatus: 400, wantText: "no"},
		{name: "numeric 401", frame: `{"error":{"message":"no","code":401}}`, wantStatus: 401, wantText: "no"},
		{name: "numeric 402", frame: `{"error":{"message":"no","code":402}}`, wantStatus: 402, wantText: "no"},
		{name: "numeric 403", frame: `{"error":{"message":"no","code":403}}`, wantStatus: 403, wantText: "no"},
		{name: "numeric 404", frame: `{"error":{"message":"no","code":404}}`, wantStatus: 404, wantText: "no"},
		{name: "string 422", frame: `{"error":{"message":"no","code":"422"}}`, wantStatus: 422, wantText: "no"},
		{name: "invalid api key", frame: `{"error":{"message":"no","code":"invalid_api_key"}}`, wantStatus: 401, wantText: "no"},
		{name: "invalid request", frame: `{"error":{"message":"no","type":"invalid_request_error","code":null}}`, wantStatus: 400, wantText: "no"},
		{name: "insufficient quota", frame: `{"error":{"message":"no","type":"insufficient_quota"}}`, wantStatus: 402, wantText: "no"},
		{name: "quota sent as 429", frame: `{"error":{"message":"no","type":"insufficient_quota","code":429}}`, wantStatus: 402, wantText: "no"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := readAll(context.Background(), strings.NewReader(sseBody(chunkText("par"), tc.frame)))
			if err == nil || !strings.Contains(err.Error(), tc.wantText) {
				t.Fatalf("err = %v", err)
			}
			if errors.Is(err, ErrTruncated) {
				t.Fatalf("misreported as truncation: %v", err)
			}
			var rl *provider.RateLimitError
			var te *provider.TransientError
			var he *provider.HTTPError
			if tc.wantStatus != 0 {
				if !errors.As(err, &he) || he.Status != tc.wantStatus || errors.As(err, &te) || errors.As(err, &rl) {
					t.Fatalf("err = %#v", err)
				}
				return
			}
			if tc.wantRL {
				if !errors.As(err, &rl) || rl.Overloaded != tc.overloaded {
					t.Fatalf("err = %#v", err)
				}
				return
			}
			if !errors.As(err, &te) {
				t.Fatalf("err = %#v", err)
			}
		})
	}
}

func TestStreamNullErrorIgnored(t *testing.T) {
	frame := `{"error":null,"choices":[{"delta":{"content":"ok"},"finish_reason":"stop"}]}`
	if err := readAll(context.Background(), strings.NewReader(sseDone(frame))); err != nil {
		t.Fatal(err)
	}
}

func TestStreamReadFailure(t *testing.T) {
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	whole := "data: " + chunkText("par") + "\n\n"
	partial := `data: {"choices":[{"del`
	tests := []struct {
		name          string
		ctx           context.Context
		body          io.Reader
		wantTransient bool
		wantIs        error
	}{
		{
			name:          "unexpected eof",
			ctx:           context.Background(),
			body:          io.MultiReader(strings.NewReader(whole), iotest.ErrReader(io.ErrUnexpectedEOF)),
			wantTransient: true,
			wantIs:        io.ErrUnexpectedEOF,
		},
		{
			name:          "reset inside a frame",
			ctx:           context.Background(),
			body:          io.MultiReader(strings.NewReader(partial), iotest.ErrReader(io.ErrUnexpectedEOF)),
			wantTransient: true,
		},
		{
			name:          "clean eof inside a frame",
			ctx:           context.Background(),
			body:          strings.NewReader(partial),
			wantTransient: true,
		},
		{
			name:   "cancelled read",
			ctx:    cancelled,
			body:   io.MultiReader(strings.NewReader(whole), iotest.ErrReader(io.ErrUnexpectedEOF)),
			wantIs: context.Canceled,
		},
		{
			name:   "cancelled inside a frame",
			ctx:    cancelled,
			body:   strings.NewReader(partial),
			wantIs: context.Canceled,
		},
		{
			name:   "oversized line",
			ctx:    context.Background(),
			body:   strings.NewReader("data: " + strings.Repeat("x", sseMaxLine+1) + "\n"),
			wantIs: bufio.ErrTooLong,
		},
		{
			name: "malformed complete frame",
			ctx:  context.Background(),
			body: strings.NewReader("data: not-json\n\n"),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := readAll(tc.ctx, tc.body)
			if err == nil {
				t.Fatal("expected error")
			}
			var te *provider.TransientError
			if errors.As(err, &te) != tc.wantTransient {
				t.Fatalf("err = %#v", err)
			}
			if tc.wantIs != nil && !errors.Is(err, tc.wantIs) {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestStreamRequestFailure(t *testing.T) {
	errDial := errors.New("connection refused")
	tests := []struct {
		name          string
		cancelInDo    bool
		wantTransient bool
		wantIs        error
	}{
		{name: "connection error", wantTransient: true, wantIs: errDial},
		{name: "cancelled during do", cancelInDo: true, wantIs: context.Canceled},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			transport := roundTripFunc(func(*http.Request) (*http.Response, error) {
				if tc.cancelInDo {
					cancel()
				}
				return nil, errDial
			})
			c := &Client{BaseURL: "http://hp.invalid", HTTP: &http.Client{Transport: transport}}
			_, err := collectAll(c.Stream(ctx, provider.Request{Model: "m"}))
			var te *provider.TransientError
			if errors.As(err, &te) != tc.wantTransient || !errors.Is(err, tc.wantIs) {
				t.Fatalf("err = %#v", err)
			}
		})
	}
}

func TestStreamConnectionDiesMidBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		io.WriteString(conn, "HTTP/1.1 200 OK\r\nTransfer-Encoding: chunked\r\n\r\n")
		line := "data: " + chunkText("part") + "\n\n"
		io.WriteString(conn, fmt.Sprintf("%x\r\n%s\r\n", len(line), line))
		conn.Close()
	}))
	defer srv.Close()
	_, _, err := provider.Collect(testClient(srv, "").Stream(context.Background(), provider.Request{Model: "m"}))
	var te *provider.TransientError
	if !errors.As(err, &te) || !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("err = %#v", err)
	}
}
