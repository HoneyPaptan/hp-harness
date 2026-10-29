package openai

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

func sseBody(chunks ...string) string {
	var b strings.Builder
	for _, c := range chunks {
		b.WriteString("data: ")
		b.WriteString(c)
		b.WriteString("\n\n")
	}
	return b.String()
}

func sseDone(chunks ...string) string {
	return sseBody(chunks...) + "data: [DONE]\n\n"
}

func collectAll(seq func(yield func(provider.Event, error) bool)) ([]provider.Event, error) {
	var out []provider.Event
	for ev, err := range seq {
		if err != nil {
			return out, err
		}
		out = append(out, ev)
	}
	return out, nil
}

func chunkText(content string) string {
	return `{"choices":[{"delta":{"content":"` + content + `"}}]}`
}

func chunkTool(index int, id, name, args string) string {
	type fn struct {
		Name      string `json:"name,omitempty"`
		Arguments string `json:"arguments,omitempty"`
	}
	type tc struct {
		Index    int    `json:"index"`
		ID       string `json:"id,omitempty"`
		Function fn     `json:"function"`
	}
	type delta struct {
		ToolCalls []tc `json:"tool_calls"`
	}
	type choice struct {
		Delta delta `json:"delta"`
	}
	type payload struct {
		Choices []choice `json:"choices"`
	}
	b, _ := json.Marshal(payload{Choices: []choice{{Delta: delta{ToolCalls: []tc{{Index: index, ID: id, Function: fn{Name: name, Arguments: args}}}}}}})
	return string(b)
}

func chunkFinish(reason string) string {
	return `{"choices":[{"delta":{},"finish_reason":"` + reason + `"}]}`
}

func chunkUsage(in, out int) string {
	return `{"choices":[],"usage":{"prompt_tokens":` + itoa(in) + `,"completion_tokens":` + itoa(out) + `}}`
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

func testClient(srv *httptest.Server, key string) *Client {
	return &Client{BaseURL: srv.URL, APIKey: key, HTTP: srv.Client()}
}

func TestStreamTextOrder(t *testing.T) {
	body := sseDone(chunkText("a"), chunkText("b"), chunkFinish("stop"))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, body)
	}))
	defer srv.Close()
	c := testClient(srv, "")
	events, err := collectAll(c.Stream(context.Background(), provider.Request{Model: "m"}))
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 {
		t.Fatalf("events = %d", len(events))
	}
	if events[0].Kind != provider.EventText || events[0].Text != "a" {
		t.Fatalf("ev0 = %+v", events[0])
	}
	if events[1].Kind != provider.EventText || events[1].Text != "b" {
		t.Fatalf("ev1 = %+v", events[1])
	}
	if events[2].Kind != provider.EventDone || events[2].StopReason != "stop" {
		t.Fatalf("ev2 = %+v", events[2])
	}
	msg, _, err := provider.Collect(c.Stream(context.Background(), provider.Request{Model: "m"}))
	if err != nil {
		t.Fatal(err)
	}
	if msg.Text != "ab" {
		t.Fatalf("text = %q", msg.Text)
	}
}

func TestStreamSplitToolArgs(t *testing.T) {
	body := sseDone(
		chunkTool(0, "c1", "read", `{"pa`),
		chunkTool(0, "", "", `th":"a.go"}`),
		chunkFinish("tool_calls"),
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, body)
	}))
	defer srv.Close()
	msg, _, err := provider.Collect(testClient(srv, "").Stream(context.Background(), provider.Request{Model: "m"}))
	if err != nil {
		t.Fatal(err)
	}
	if len(msg.ToolCalls) != 1 {
		t.Fatalf("calls = %+v", msg.ToolCalls)
	}
	want := `{"path":"a.go"}`
	if string(msg.ToolCalls[0].Args) != want {
		t.Fatalf("args = %s", msg.ToolCalls[0].Args)
	}
}

func TestStreamParallelTools(t *testing.T) {
	body := sseDone(
		chunkTool(0, "a", "one", `{}`),
		chunkTool(1, "b", "two", `{}`),
		chunkFinish("tool_calls"),
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, body)
	}))
	defer srv.Close()
	msg, _, err := provider.Collect(testClient(srv, "").Stream(context.Background(), provider.Request{Model: "m"}))
	if err != nil {
		t.Fatal(err)
	}
	if len(msg.ToolCalls) != 2 || msg.ToolCalls[0].ID != "a" || msg.ToolCalls[1].ID != "b" {
		t.Fatalf("calls = %+v", msg.ToolCalls)
	}
}

func TestStreamUsageEmptyChoices(t *testing.T) {
	body := sseDone(chunkText("x"), chunkUsage(5, 2), chunkFinish("stop"))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, body)
	}))
	defer srv.Close()
	_, use, err := provider.Collect(testClient(srv, "").Stream(context.Background(), provider.Request{Model: "m"}))
	if err != nil {
		t.Fatal(err)
	}
	if use.InputTokens != 5 || use.OutputTokens != 2 {
		t.Fatalf("usage = %+v", use)
	}
}

func TestStreamNoDoneMarker(t *testing.T) {
	body := sseBody(chunkText("ok"), chunkFinish("stop"))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, body)
	}))
	defer srv.Close()
	msg, _, err := provider.Collect(testClient(srv, "").Stream(context.Background(), provider.Request{Model: "m"}))
	if err != nil {
		t.Fatal(err)
	}
	if msg.Text != "ok" {
		t.Fatalf("text = %q", msg.Text)
	}
}

func TestStreamTruncatedWithoutFinish(t *testing.T) {
	body := sseBody(chunkText("par"))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, body)
	}))
	defer srv.Close()
	_, _, err := provider.Collect(testClient(srv, "").Stream(context.Background(), provider.Request{Model: "m"}))
	if !errors.Is(err, ErrTruncated) {
		t.Fatalf("err = %v", err)
	}
}

func TestStreamHTTP400(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		io.WriteString(w, `{"error":{"message":"bad request"}}`)
	}))
	defer srv.Close()
	_, _, err := provider.Collect(testClient(srv, "").Stream(context.Background(), provider.Request{Model: "m"}))
	var he *provider.HTTPError
	if !errors.As(err, &he) {
		t.Fatalf("err = %v", err)
	}
	if he.Status != 400 || !strings.Contains(he.Body, "bad request") {
		t.Fatalf("he = %+v", he)
	}
}

func TestStreamHTTP429(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "30")
		w.WriteHeader(429)
		io.WriteString(w, "rate limited")
	}))
	defer srv.Close()
	_, _, err := provider.Collect(testClient(srv, "").Stream(context.Background(), provider.Request{Model: "m"}))
	var rl *provider.RateLimitError
	if !errors.As(err, &rl) {
		t.Fatalf("err = %v", err)
	}
	if rl.RetryAfter != 30*time.Second {
		t.Fatalf("retry = %v", rl.RetryAfter)
	}
}

func TestStreamBrokenBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Fatal("no hijack")
		}
		conn, _, err := hj.Hijack()
		if err != nil {
			t.Fatal(err)
		}
		conn.Write([]byte("data: " + chunkText("part") + "\n\n"))
		conn.Close()
	}))
	defer srv.Close()
	_, _, err := provider.Collect(testClient(srv, "").Stream(context.Background(), provider.Request{Model: "m"}))
	if err == nil {
		t.Fatal("expected error")
	}
	if errors.Is(err, provider.ErrNoDone) {
		t.Fatal("got ErrNoDone")
	}
}

func TestStreamContextCancel(t *testing.T) {
	block := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-block
	}))
	t.Cleanup(func() {
		close(block)
		srv.CloseClientConnections()
		srv.Close()
	})
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		_, _, err := provider.Collect(testClient(srv, "").Stream(ctx, provider.Request{Model: "m"}))
		errCh <- err
	}()
	time.Sleep(20 * time.Millisecond)
	cancel()
	select {
	case err := <-errCh:
		if err == nil || !errors.Is(err, context.Canceled) {
			t.Fatalf("err = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for cancel")
	}
}

func TestStreamAuthHeader(t *testing.T) {
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		io.WriteString(w, sseDone(chunkFinish("stop")))
	}))
	defer srv.Close()
	_, _, _ = provider.Collect(testClient(srv, "").Stream(context.Background(), provider.Request{Model: "m"}))
	if auth != "" {
		t.Fatalf("auth = %q", auth)
	}
	_, _, _ = provider.Collect(testClient(srv, "sk-x").Stream(context.Background(), provider.Request{Model: "m"}))
	if auth != "Bearer sk-x" {
		t.Fatalf("auth = %q", auth)
	}
}

func TestStreamRequestBody(t *testing.T) {
	var posted json.RawMessage
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		b, _ := io.ReadAll(r.Body)
		posted = b
		io.WriteString(w, sseDone(chunkFinish("stop")))
	}))
	defer srv.Close()
	_, _, _ = provider.Collect(testClient(srv, "").Stream(context.Background(), provider.Request{Model: "m"}))
	var body map[string]any
	if json.Unmarshal(posted, &body) != nil {
		t.Fatal("unmarshal posted body")
	}
	if body["stream"] != true {
		t.Fatalf("stream = %v", body["stream"])
	}
	opts, _ := body["stream_options"].(map[string]any)
	if opts["include_usage"] != true {
		t.Fatalf("stream_options = %v", body["stream_options"])
	}
}

func TestStreamBadJSONChunk(t *testing.T) {
	body := sseDone("not-json", chunkFinish("stop"))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, body)
	}))
	defer srv.Close()
	_, _, err := provider.Collect(testClient(srv, "").Stream(context.Background(), provider.Request{Model: "m"}))
	if err == nil {
		t.Fatal("expected decode error")
	}
}

func TestStreamEmptyToolArgs(t *testing.T) {
	body := sseDone(chunkTool(0, "c", "f", ""), chunkFinish("tool_calls"))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, body)
	}))
	defer srv.Close()
	msg, _, err := provider.Collect(testClient(srv, "").Stream(context.Background(), provider.Request{Model: "m"}))
	if err != nil {
		t.Fatal(err)
	}
	if string(msg.ToolCalls[0].Args) != "{}" {
		t.Fatalf("args = %s", msg.ToolCalls[0].Args)
	}
}

func TestStreamBaseURLTrim(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, sseDone(chunkFinish("stop")))
	}))
	defer srv.Close()
	c := &Client{BaseURL: srv.URL + "/", HTTP: srv.Client()}
	_, _, err := provider.Collect(c.Stream(context.Background(), provider.Request{Model: "m"}))
	if err != nil {
		t.Fatal(err)
	}
}
