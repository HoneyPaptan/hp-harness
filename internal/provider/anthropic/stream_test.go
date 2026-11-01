package anthropic

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/HoneyPaptan/hp-harness/internal/provider"
)

const (
	evStart   = `{"type":"message_start","message":{"usage":{"input_tokens":10,"cache_read_input_tokens":100,"cache_creation_input_tokens":20,"output_tokens":1}}}`
	evBlock   = `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`
	evPing    = `{"type":"ping"}`
	evStop    = `{"type":"content_block_stop","index":0}`
	evDelta   = `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":7}}`
	evDone    = `{"type":"message_stop"}`
	evUnknown = `{"type":"content_block_start","index":9,"content_block":{"type":"fallback"}}`
)

func textDelta(text string) string {
	return `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"` + text + `"}}`
}

func sse(events ...string) string {
	var b strings.Builder
	for _, ev := range events {
		var head struct {
			Type string `json:"type"`
		}
		_ = json.Unmarshal([]byte(ev), &head)
		b.WriteString("event: " + head.Type + "\ndata: " + ev + "\n\n")
	}
	return b.String()
}

func serve(t *testing.T, status int, header map[string]string, body string) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		for k, v := range header {
			w.Header().Set(k, v)
		}
		w.WriteHeader(status)
		io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	return &Client{BaseURL: srv.URL + "/v1/", APIKey: "k", HTTP: srv.Client()}
}

func run(c *Client) ([]provider.Event, error) {
	var out []provider.Event
	for ev, err := range c.Stream(context.Background(), provider.Request{Model: "m"}) {
		if err != nil {
			return out, err
		}
		out = append(out, ev)
	}
	return out, nil
}

func TestStreamText(t *testing.T) {
	c := serve(t, 200, nil, sse(evStart, evBlock, evPing, textDelta("he"), textDelta("llo"), evStop, evDelta, evDone))
	msg, usage, err := provider.Collect(c.Stream(context.Background(), provider.Request{Model: "m"}))
	if err != nil {
		t.Fatal(err)
	}
	if msg.Text != "hello" {
		t.Fatalf("text = %q", msg.Text)
	}
	if usage.InputTokens != 130 || usage.OutputTokens != 7 {
		t.Fatalf("usage = %+v", usage)
	}
}

func TestStreamSkipsThinkingAndUnknownBlocks(t *testing.T) {
	c := serve(t, 200, nil, sse(
		evStart, evUnknown,
		`{"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":""}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"secret"}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"s"}}`,
		evStop,
		`{"type":"content_block_start","index":1,"content_block":{"type":"text","text":""}}`,
		`{"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"answer"}}`,
		evDelta, evDone,
	))
	msg, _, err := provider.Collect(c.Stream(context.Background(), provider.Request{Model: "m"}))
	if err != nil {
		t.Fatal(err)
	}
	if msg.Text != "answer" {
		t.Fatalf("text = %q", msg.Text)
	}
}

func TestStreamUsageRefreshedByMessageDelta(t *testing.T) {
	delta := `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"input_tokens":11,"cache_read_input_tokens":200,"output_tokens":3}}`
	c := serve(t, 200, nil, sse(evStart, delta, evDone))
	_, usage, err := provider.Collect(c.Stream(context.Background(), provider.Request{Model: "m"}))
	if err != nil {
		t.Fatal(err)
	}
	if usage.InputTokens != 231 || usage.OutputTokens != 3 {
		t.Fatalf("usage = %+v", usage)
	}
}

func TestToolRoundTrip(t *testing.T) {
	first := sse(
		evStart,
		`{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_1","name":"read","input":{}}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":""}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"path\":"}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"\"a\"}"}}`,
		evStop,
		`{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"toolu_2","name":"list","input":{}}}`,
		`{"type":"content_block_stop","index":1}`,
		`{"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":4}}`,
		evDone,
	)
	second := sse(evStart, evBlock, textDelta("done"), evStop, evDelta, evDone)
	var bodies []wireRequest
	var headers []http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req wireRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
		}
		bodies = append(bodies, req)
		headers = append(headers, r.Header.Clone())
		if len(bodies) == 1 {
			io.WriteString(w, first)
			return
		}
		io.WriteString(w, second)
	}))
	defer srv.Close()
	c := &Client{BaseURL: srv.URL + "/v1", APIKey: "secret", HTTP: srv.Client()}

	req := provider.Request{Model: "m", Messages: []provider.Message{{Role: provider.RoleUser, Text: "go"}}}
	msg, _, err := provider.Collect(c.Stream(context.Background(), req))
	if err != nil {
		t.Fatal(err)
	}
	if len(msg.ToolCalls) != 2 {
		t.Fatalf("calls = %+v", msg.ToolCalls)
	}
	if got := string(msg.ToolCalls[0].Args); got != `{"path":"a"}` {
		t.Fatalf("args = %s", got)
	}
	if got := string(msg.ToolCalls[1].Args); got != `{}` {
		t.Fatalf("empty args = %s", got)
	}

	req.Messages = append(req.Messages, msg, provider.Message{Role: provider.RoleUser, ToolResults: []provider.ToolResult{
		{CallID: "toolu_1", Text: "file"}, {CallID: "toolu_2", Text: "boom", IsError: true},
	}})
	msg, _, err = provider.Collect(c.Stream(context.Background(), req))
	if err != nil {
		t.Fatal(err)
	}
	if msg.Text != "done" {
		t.Fatalf("text = %q", msg.Text)
	}

	sent := bodies[1].Messages
	if len(sent) != 3 || sent[1].Role != "assistant" || sent[1].Content[0].Type != "tool_use" {
		t.Fatalf("history = %+v", sent)
	}
	results := sent[2].Content
	if results[0].ToolUseID != "toolu_1" || results[1].ToolUseID != "toolu_2" || !results[1].IsError {
		t.Fatalf("results = %+v", results)
	}
	if results[1].CacheControl == nil {
		t.Fatal("last user block has no breakpoint")
	}
	h := headers[0]
	if h.Get("x-api-key") != "secret" || h.Get("anthropic-version") != apiVersion || h.Get("Authorization") != "" {
		t.Fatalf("headers = %v", h)
	}
}

func TestStreamTruncation(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{name: "no events", body: ""},
		{name: "cut mid text", body: sse(evStart, evBlock, textDelta("he"))},
		{name: "message delta without stop", body: sse(evStart, evDelta)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := run(serve(t, 200, nil, tc.body))
			var te *provider.TransientError
			if !errors.As(err, &te) || !errors.Is(err, ErrTruncated) {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func TestInStreamErrors(t *testing.T) {
	cases := []struct {
		kind       string
		overloaded bool
		limited    bool
		status     int
	}{
		{kind: "overloaded_error", overloaded: true, limited: true},
		{kind: "rate_limit_error", limited: true},
		{kind: "invalid_request_error", status: 400},
		{kind: "authentication_error", status: 401},
		{kind: "not_found_error", status: 404},
		{kind: "api_error"},
		{kind: "something_new"},
	}
	for _, tc := range cases {
		t.Run(tc.kind, func(t *testing.T) {
			body := sse(evStart, `{"type":"error","error":{"type":"`+tc.kind+`","message":"bad"}}`)
			_, err := run(serve(t, 200, nil, body))
			assertKind(t, err, tc.limited, tc.overloaded, tc.status)
		})
	}
}

func assertKind(t *testing.T, err error, limited, overloaded bool, status int) {
	t.Helper()
	var rl *provider.RateLimitError
	var he *provider.HTTPError
	var te *provider.TransientError
	switch {
	case limited:
		if !errors.As(err, &rl) || rl.Overloaded != overloaded {
			t.Fatalf("err = %#v", err)
		}
	case status != 0:
		if errors.As(err, &te) || !errors.As(err, &he) || he.Status != status {
			t.Fatalf("err = %#v", err)
		}
	default:
		if !errors.As(err, &te) {
			t.Fatalf("err = %#v", err)
		}
	}
}

func TestHTTPErrors(t *testing.T) {
	cases := []struct {
		status     int
		limited    bool
		overloaded bool
		permanent  int
	}{
		{status: 429, limited: true},
		{status: 529, limited: true, overloaded: true},
		{status: 408},
		{status: 409},
		{status: 500},
		{status: 503},
		{status: 400, permanent: 400},
		{status: 401, permanent: 401},
		{status: 404, permanent: 404},
	}
	for _, tc := range cases {
		t.Run(http.StatusText(tc.status), func(t *testing.T) {
			_, err := run(serve(t, tc.status, nil, "nope"))
			assertKind(t, err, tc.limited, tc.overloaded, tc.permanent)
		})
	}
}

func TestRateLimitDelay(t *testing.T) {
	fixed := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	oldNow := now
	now = func() time.Time { return fixed }
	t.Cleanup(func() { now = oldNow })
	reset := func(d time.Duration) string { return fixed.Add(d).Format(time.RFC3339) }
	cases := []struct {
		name   string
		header map[string]string
		want   time.Duration
	}{
		{name: "retry after wins", header: map[string]string{
			"retry-after": "3", "anthropic-ratelimit-tokens-reset": reset(time.Minute),
		}, want: 3 * time.Second},
		{name: "latest reset", header: map[string]string{
			"anthropic-ratelimit-requests-reset":      reset(10 * time.Second),
			"anthropic-ratelimit-tokens-reset":        reset(40 * time.Second),
			"anthropic-ratelimit-output-tokens-reset": reset(20 * time.Second),
		}, want: 40 * time.Second},
		{name: "past reset", header: map[string]string{
			"anthropic-ratelimit-tokens-reset": reset(-time.Minute),
		}},
		{name: "unreadable retry after falls to reset", header: map[string]string{
			"retry-after": "soon", "anthropic-ratelimit-input-tokens-reset": reset(5 * time.Second),
		}, want: 5 * time.Second},
		{name: "no headers"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := run(serve(t, 429, tc.header, "slow"))
			var rl *provider.RateLimitError
			if !errors.As(err, &rl) || rl.RetryAfter != tc.want {
				t.Fatalf("err = %#v, want delay %v", err, tc.want)
			}
		})
	}
}

func TestDroppedConnectionIsTransient(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	client := srv.Client()
	srv.Close()
	_, err := run(&Client{BaseURL: url, HTTP: client})
	var te *provider.TransientError
	if !errors.As(err, &te) {
		t.Fatalf("err = %#v", err)
	}
}

func TestMalformedEventIsPermanent(t *testing.T) {
	_, err := run(serve(t, 200, nil, "data: {\"type\":]} trailing text\n\n"))
	var te *provider.TransientError
	if err == nil || errors.As(err, &te) {
		t.Fatalf("err = %#v", err)
	}
}

func TestCancelledContextReturnsCause(t *testing.T) {
	c := serve(t, 200, nil, "")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, err := range c.Stream(ctx, provider.Request{Model: "m"}) {
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("err = %v", err)
		}
	}
}
