package anthropic

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/HoneyPaptan/hp-harness/internal/provider"
	"github.com/HoneyPaptan/hp-harness/internal/provider/transport"
)

func TestThinkingBlocksReportReasoning(t *testing.T) {
	cases := []struct {
		name   string
		events []string
		want   []string
	}{
		{
			name: "summarized",
			events: []string{
				`{"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":""}}`,
				`{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"hm"}}`,
			},
			want: []string{"", "hm"},
		},
		{
			name:   "omitted display",
			events: []string{`{"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":""}}`},
			want:   []string{""},
		},
		{
			name:   "redacted",
			events: []string{`{"type":"content_block_start","index":0,"content_block":{"type":"redacted_thinking","data":"x"}}`},
			want:   []string{""},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := run(serve(t, 200, nil, sse(append(append([]string{evStart}, tc.events...), evDelta, evDone)...)))
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != len(tc.want)+1 {
				t.Fatalf("events = %+v", got)
			}
			for i, text := range tc.want {
				if got[i].Kind != provider.EventReasoning || got[i].Text != text {
					t.Fatalf("event %d = %+v", i, got[i])
				}
			}
		})
	}
}

func TestFirstPromptReusesThePreconnectedConnection(t *testing.T) {
	var conns atomic.Int32
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			io.WriteString(w, sse(evStart, evDelta, evDone))
		}
	}))
	srv.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateNew {
			conns.Add(1)
		}
	}
	srv.Start()
	defer srv.Close()
	c := &Client{BaseURL: srv.URL + "/v1", HTTP: transport.NewHTTPClient()}
	c.Preconnect(context.Background())
	for range 2 {
		if _, _, err := provider.Collect(c.Stream(context.Background(), provider.Request{Model: "m"})); err != nil {
			t.Fatal(err)
		}
	}
	if got := conns.Load(); got != 1 {
		t.Fatalf("connections = %d", got)
	}
}
