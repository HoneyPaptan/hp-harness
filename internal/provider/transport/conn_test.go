package transport

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func countingServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var conns atomic.Int32
	srv := httptest.NewUnstartedServer(handler)
	srv.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateNew {
			conns.Add(1)
		}
	}
	srv.Start()
	t.Cleanup(srv.Close)
	return srv, &conns
}

func TestPreconnectLeavesAReusableConnection(t *testing.T) {
	methods := make(chan string, 2)
	srv, conns := countingServer(t, func(w http.ResponseWriter, r *http.Request) {
		methods <- r.Method
		w.WriteHeader(http.StatusNotFound)
	})
	client := NewHTTPClient()
	Preconnect(context.Background(), client, srv.URL+"/v1")
	resp, err := client.Post(srv.URL+"/v1/next", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if got := conns.Load(); got != 1 {
		t.Fatalf("connections = %d", got)
	}
	if first, second := <-methods, <-methods; first != http.MethodHead || second != http.MethodPost {
		t.Fatalf("methods = %s %s", first, second)
	}
}

func TestPreconnectSendsNoCredentials(t *testing.T) {
	seen := make(chan http.Header, 1)
	srv, _ := countingServer(t, func(_ http.ResponseWriter, r *http.Request) { seen <- r.Header.Clone() })
	Preconnect(context.Background(), NewHTTPClient(), srv.URL)
	h := <-seen
	if h.Get("Authorization") != "" || h.Get("x-api-key") != "" {
		t.Fatalf("headers = %v", h)
	}
}

func TestPreconnectIgnoresRedirects(t *testing.T) {
	var hits atomic.Int32
	srv, _ := countingServer(t, func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.Header().Set("Location", "/elsewhere")
		w.WriteHeader(http.StatusFound)
	})
	Preconnect(context.Background(), NewHTTPClient(), srv.URL)
	if got := hits.Load(); got != 1 {
		t.Fatalf("hits = %d", got)
	}
}

func TestPreconnectFailuresStaySilent(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	dead := srv.URL
	srv.Close()
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	cases := []struct {
		name string
		ctx  context.Context
		url  string
	}{
		{name: "refused", ctx: context.Background(), url: dead},
		{name: "bad url", ctx: context.Background(), url: "http://\x7f"},
		{name: "cancelled", ctx: cancelled, url: dead},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			start := time.Now()
			Preconnect(tc.ctx, NewHTTPClient(), tc.url)
			if time.Since(start) > preconnectWait {
				t.Fatal("preconnect overran its wait")
			}
		})
	}
}

func TestNewHTTPClientOwnsItsTransport(t *testing.T) {
	a, b := NewHTTPClient(), NewHTTPClient()
	if a.Transport == b.Transport || a.Transport == http.DefaultTransport {
		t.Fatal("transports are shared")
	}
}
