package openai

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/HoneyPaptan/hp-harness/internal/provider"
)

func streamBody(t *testing.T, body string) (provider.Message, error) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, body)
	}))
	defer srv.Close()
	msg, _, err := provider.Collect(testClient(srv, "").Stream(context.Background(), provider.Request{Model: "m"}))
	return msg, err
}

func TestFreeModelArgsAsObject(t *testing.T) {
	chunk := `{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"c1","function":{"name":"read","arguments":{"path":"a.go"}}}]}}]}`
	msg, err := streamBody(t, sseDone(chunk, chunkFinish("tool_calls")))
	if err != nil {
		t.Fatal(err)
	}
	if len(msg.ToolCalls) != 1 || string(msg.ToolCalls[0].Args) != `{"path":"a.go"}` {
		t.Fatalf("calls = %+v", msg.ToolCalls)
	}
}

func TestFreeModelMissingCallIDs(t *testing.T) {
	msg, err := streamBody(t, sseDone(chunkTool(0, "", "read", `{}`), chunkTool(1, "", "list", ""), chunkFinish("tool_calls")))
	if err != nil {
		t.Fatal(err)
	}
	if len(msg.ToolCalls) != 2 {
		t.Fatalf("calls = %+v", msg.ToolCalls)
	}
	a, b := msg.ToolCalls[0], msg.ToolCalls[1]
	if a.ID == "" || b.ID == "" || a.ID == b.ID {
		t.Fatalf("ids = %q %q", a.ID, b.ID)
	}
	if string(b.Args) != "{}" {
		t.Fatalf("empty args = %s", b.Args)
	}
}

func TestFreeModelTextToolCall(t *testing.T) {
	msg, err := streamBody(t, sseDone(
		chunkText("Let me look.<tool"),
		chunkText(`_call>{\"name\":\"read\",\"arguments\":{\"path\":\"a.go\"}}</tool_call>`),
		chunkFinish("stop"),
	))
	if err != nil {
		t.Fatal(err)
	}
	if msg.Text != "Let me look." {
		t.Fatalf("text = %q", msg.Text)
	}
	if len(msg.ToolCalls) != 1 || msg.ToolCalls[0].Name != "read" || string(msg.ToolCalls[0].Args) != `{"path":"a.go"}` || msg.ToolCalls[0].ID == "" {
		t.Fatalf("calls = %+v", msg.ToolCalls)
	}
}

func TestFreeModelTextToolCallStringArgs(t *testing.T) {
	msg, err := streamBody(t, sseDone(chunkText(`<tool_call>{\"name\":\"grep\",\"arguments\":\"{\\\"pattern\\\":\\\"x\\\"}\"}`), chunkFinish("stop")))
	if err != nil {
		t.Fatal(err)
	}
	if len(msg.ToolCalls) != 1 || string(msg.ToolCalls[0].Args) != `{"pattern":"x"}` {
		t.Fatalf("calls = %+v", msg.ToolCalls)
	}
}

func TestFreeModelBrokenTextCallStaysText(t *testing.T) {
	msg, err := streamBody(t, sseDone(chunkText("see <tool_call>not json</tool_call>"), chunkFinish("stop")))
	if err != nil {
		t.Fatal(err)
	}
	if msg.Text != "see <tool_call>not json</tool_call>" || len(msg.ToolCalls) != 0 {
		t.Fatalf("msg = %+v", msg)
	}
}

func TestFreeModelHeldPrefixIsFlushed(t *testing.T) {
	msg, err := streamBody(t, sseDone(chunkText("a <too"), chunkFinish("stop")))
	if err != nil {
		t.Fatal(err)
	}
	if msg.Text != "a <too" {
		t.Fatalf("text = %q", msg.Text)
	}
}

func TestFreeModelStructuredCallsWinOverText(t *testing.T) {
	msg, err := streamBody(t, sseDone(chunkText(`<tool_call>{\"name\":\"list\"}</tool_call>`), chunkTool(0, "c1", "read", `{}`), chunkFinish("tool_calls")))
	if err != nil {
		t.Fatal(err)
	}
	if len(msg.ToolCalls) != 1 || msg.ToolCalls[0].Name != "read" {
		t.Fatalf("calls = %+v", msg.ToolCalls)
	}
}

func TestFreeModelDoneWithoutFinishReason(t *testing.T) {
	msg, err := streamBody(t, sseDone(chunkText("ok")))
	if err != nil {
		t.Fatal(err)
	}
	if msg.Text != "ok" {
		t.Fatalf("text = %q", msg.Text)
	}
}

func TestFreeModelEmptyDoneIsTruncated(t *testing.T) {
	_, err := streamBody(t, sseDone())
	if !errors.Is(err, ErrTruncated) {
		t.Fatalf("err = %v", err)
	}
}

func TestRateLimitResetEpochMillis(t *testing.T) {
	fixed := time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)
	old := now
	now = func() time.Time { return fixed }
	defer func() { now = old }()
	cases := map[string]time.Duration{
		"1794744030000": 30 * time.Second,
		"1794744045":    45 * time.Second,
		"12":            12 * time.Second,
		"soon":          0,
	}
	for v, want := range cases {
		h := http.Header{}
		h.Set("X-RateLimit-Reset", v)
		if got := retryAfterFromHeaders(h); got != want {
			t.Fatalf("%s: got %v, want %v", v, got, want)
		}
	}
}

func TestSSECommentsIgnored(t *testing.T) {
	msg, err := streamBody(t, ": OPENROUTER PROCESSING\n\n"+sseDone(chunkText("hi"), chunkFinish("stop")))
	if err != nil || !strings.Contains(msg.Text, "hi") {
		t.Fatalf("msg = %+v, err = %v", msg, err)
	}
}
