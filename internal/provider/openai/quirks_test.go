package openai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/HoneyPaptan/hp-harness/internal/provider"
)

func TestRequestQuirks(t *testing.T) {
	cases := []struct {
		name         string
		client       Client
		wantUsage    bool
		wantThinking bool
	}{
		{name: "default", wantUsage: true},
		{name: "no stream options", client: Client{NoStreamOptions: true}},
		{name: "thinking off", client: Client{DisableThinking: true}, wantUsage: true, wantThinking: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b, err := tc.client.encodeStreamRequest(provider.Request{Model: "m"})
			if err != nil {
				t.Fatal(err)
			}
			body := string(b)
			if strings.Contains(body, "stream_options") != tc.wantUsage {
				t.Fatalf("stream_options wrong: %s", body)
			}
			if strings.Contains(body, `"thinking":{"type":"disabled"}`) != tc.wantThinking {
				t.Fatalf("thinking wrong: %s", body)
			}
		})
	}
}

func TestDeltaContentShapes(t *testing.T) {
	cases := []struct {
		name  string
		chunk string
		want  string
	}{
		{name: "string", chunk: `{"choices":[{"delta":{"content":"hi"}}]}`, want: "hi"},
		{name: "null", chunk: `{"choices":[{"delta":{"content":null}}]}`},
		{name: "chunks", chunk: `{"choices":[{"delta":{"content":[{"type":"text","text":"a"},{"type":"thinking","thinking":[]},{"type":"text","text":"b"}]}}]}`, want: "ab"},
		{name: "object", chunk: `{"choices":[{"delta":{"content":{"odd":1}}}]}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			chunk, err := decodeChunk(tc.chunk)
			if err != nil {
				t.Fatal(err)
			}
			if got := string(chunk.Choices[0].Delta.Content); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestToolCallExtraContentRoundTrips(t *testing.T) {
	extra := `{"google":{"thought_signature":"sig"}}`
	chunk := `{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"c1","function":{"name":"read","arguments":"{}"},"extra_content":` + extra + `}]}}]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, sseDone(chunk, chunkFinish("tool_calls")))
	}))
	defer srv.Close()
	c := &Client{BaseURL: srv.URL, HTTP: srv.Client()}

	msg, _, err := provider.Collect(c.Stream(context.Background(), provider.Request{Model: "m"}))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(msg.ToolCalls[0].Extra); got != extra {
		t.Fatalf("extra = %s", got)
	}
	wire := mapAssistantToolCalls(msg.ToolCalls)
	b, err := json.Marshal(wire)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"extra_content":`+extra) {
		t.Fatalf("wire = %s", b)
	}
}
