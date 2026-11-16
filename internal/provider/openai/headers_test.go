package openai

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/HoneyPaptan/hp-harness/internal/provider"
)

func headerServer(t *testing.T) (*http.Client, <-chan http.Header) {
	t.Helper()
	seen := make(chan http.Header, 4)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen <- r.Header.Clone()
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, sseDone(chunkText("ok"), chunkFinish("stop")))
	}))
	t.Cleanup(srv.Close)
	dial := func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, srv.Listener.Addr().String())
	}
	return &http.Client{Transport: &http.Transport{DialContext: dial}}, seen
}

func TestRequestHeaders(t *testing.T) {
	cases := []struct {
		name    string
		client  Client
		session string
		want    map[string]string
		absent  []string
	}{
		{
			name:    "opencode go carries the run id and the user agent",
			client:  Client{BaseURL: "http://opencode.ai/zen/go/v1", APIKey: "k", UserAgent: "hp/1.2.3"},
			session: "run-1",
			want:    map[string]string{"x-opencode-session": "run-1", "User-Agent": "hp/1.2.3", "Authorization": "Bearer k"},
		},
		{
			name:    "opencode preset host also carries the run id",
			client:  Client{BaseURL: "http://opencode.ai/inference/openai/v1", APIKey: "k"},
			session: "run-1",
			want:    map[string]string{"x-opencode-session": "run-1"},
		},
		{
			name:   "opencode without a run id sends no session header",
			client: Client{BaseURL: "http://opencode.ai/zen/go/v1", APIKey: "k"},
			absent: []string{"x-opencode-session"},
		},
		{
			name:    "groq gets no session header",
			client:  Client{BaseURL: "http://api.groq.com/openai/v1", APIKey: "k", UserAgent: "hp/1.2.3"},
			session: "run-1",
			want:    map[string]string{"User-Agent": "hp/1.2.3"},
			absent:  []string{"x-opencode-session"},
		},
		{
			name:    "a lookalike host gets no session header",
			client:  Client{BaseURL: "http://opencode.ai.example.com/v1", APIKey: "k"},
			session: "run-1",
			absent:  []string{"x-opencode-session"},
		},
		{
			name:    "browser style client leaves the user agent alone",
			client:  Client{BaseURL: "http://openrouter.ai/api/v1", APIKey: "k", Headers: OpenRouterApp},
			session: "run-1",
			want: map[string]string{
				"HTTP-Referer":            OpenRouterApp["HTTP-Referer"],
				"X-OpenRouter-Title":      "hp",
				"X-OpenRouter-Categories": "cli-agent",
				"User-Agent":              "Go-http-client/1.1",
			},
			absent: []string{"x-opencode-session"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hc, seen := headerServer(t)
			c := tc.client
			c.HTTP = hc
			_, err := collectAll(c.Stream(context.Background(), provider.Request{Model: "m", SessionID: tc.session}))
			if err != nil {
				t.Fatal(err)
			}
			got := <-seen
			for k, v := range tc.want {
				if got.Get(k) != v {
					t.Errorf("%s = %q, want %q", k, got.Get(k), v)
				}
			}
			for _, k := range tc.absent {
				if _, ok := got[http.CanonicalHeaderKey(k)]; ok {
					t.Errorf("%s was sent", k)
				}
			}
		})
	}
}

func TestOpenRouterHeaderSetIsUnchanged(t *testing.T) {
	hc, seen := headerServer(t)
	c := Client{BaseURL: "http://openrouter.ai/api/v1", APIKey: "k", HTTP: hc, Headers: OpenRouterApp}
	if _, err := collectAll(c.Stream(context.Background(), provider.Request{Model: "m", SessionID: "run-1"})); err != nil {
		t.Fatal(err)
	}
	got := <-seen
	want := map[string]bool{
		"Accept": true, "Accept-Encoding": true, "Authorization": true, "Content-Length": true,
		"Content-Type": true, "Http-Referer": true, "User-Agent": true,
		"X-Openrouter-Categories": true, "X-Openrouter-Title": true,
	}
	for k := range got {
		if !want[k] {
			t.Errorf("unexpected header %s", k)
		}
		delete(want, k)
	}
	for k := range want {
		t.Errorf("missing header %s", k)
	}
}

func TestListModelsUserAgent(t *testing.T) {
	hc, seen := headerServer(t)
	c := Client{BaseURL: "http://opencode.ai/zen/go/v1", HTTP: hc, UserAgent: "hp/1.2.3"}
	c.ListModels(context.Background())
	if got := (<-seen).Get("User-Agent"); got != "hp/1.2.3" {
		t.Fatalf("User-Agent = %q", got)
	}
}
