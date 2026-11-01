package openai

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

func TestReasoningDeltas(t *testing.T) {
	cases := []struct {
		name  string
		chunk string
		want  string
	}{
		{name: "reasoning", chunk: `{"choices":[{"delta":{"reasoning":"hmm"}}]}`, want: "hmm"},
		{name: "reasoning_content", chunk: `{"choices":[{"delta":{"reasoning_content":"hmm"}}]}`, want: "hmm"},
		{name: "null", chunk: `{"choices":[{"delta":{"reasoning":null,"reasoning_content":null}}]}`},
		{name: "object is ignored", chunk: `{"choices":[{"delta":{"reasoning":{"summary":"x"}}}]}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			chunk, err := decodeChunk(tc.chunk)
			if err != nil {
				t.Fatal(err)
			}
			events := newAssembler().feed(chunk)
			if tc.want == "" {
				if len(events) != 0 {
					t.Fatalf("events = %+v", events)
				}
				return
			}
			if len(events) != 1 || events[0].Kind != provider.EventReasoning || events[0].Text != tc.want {
				t.Fatalf("events = %+v", events)
			}
		})
	}
}

func TestReasoningStaysOutOfTheAnswer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, sseDone(`{"choices":[{"delta":{"reasoning":"secret"}}]}`, chunkText("answer"), chunkFinish("stop")))
	}))
	defer srv.Close()
	c := &Client{BaseURL: srv.URL, HTTP: srv.Client()}
	seq := c.Stream(context.Background(), provider.Request{Model: "m"})
	var kinds []provider.EventKind
	msg, _, err := provider.Collect(func(yield func(provider.Event, error) bool) {
		for ev, err := range seq {
			kinds = append(kinds, ev.Kind)
			if !yield(ev, err) {
				return
			}
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if msg.Text != "answer" {
		t.Fatalf("text = %q", msg.Text)
	}
	if kinds[0] != provider.EventReasoning || kinds[1] != provider.EventText {
		t.Fatalf("kinds = %v", kinds)
	}
}

func TestFirstPromptReusesThePreconnectedConnection(t *testing.T) {
	var conns atomic.Int32
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			io.WriteString(w, sseDone(chunkText("hi"), chunkFinish("stop")))
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
	if _, _, err := provider.Collect(c.Stream(context.Background(), provider.Request{Model: "m"})); err != nil {
		t.Fatal(err)
	}
	if got := conns.Load(); got != 1 {
		t.Fatalf("connections = %d", got)
	}
}
