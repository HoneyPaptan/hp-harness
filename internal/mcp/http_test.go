package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/HoneyPaptan/hp-harness/internal/mcp/mcptest"
	"github.com/HoneyPaptan/hp-harness/internal/tool"
)

type fakeHTTP struct {
	mu       sync.Mutex
	sse      bool
	session  string
	expired  bool
	ping     bool
	inits    int
	deletes  int
	seen     []http.Header
	replies  []string
	cutShort bool
}

func (f *fakeHTTP) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.seen = append(f.seen, r.Header.Clone())
	if r.Method == http.MethodDelete {
		f.deletes++
		return
	}
	body, _ := io.ReadAll(r.Body)
	var m struct {
		ID     json.RawMessage `json:"id"`
		Method string          `json:"method"`
		Params json.RawMessage `json:"params"`
		Result json.RawMessage `json:"result"`
	}
	json.Unmarshal(body, &m)
	if m.Method == "" {
		f.replies = append(f.replies, string(m.ID))
		w.WriteHeader(http.StatusAccepted)
		return
	}
	if len(m.ID) == 0 {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	f.serveRequest(w, r, m.ID, m.Method, m.Params)
}

func (f *fakeHTTP) serveRequest(w http.ResponseWriter, r *http.Request, id json.RawMessage, method string, params json.RawMessage) {
	if method == "initialize" {
		f.inits++
		f.expired = false
		if f.session != "" {
			w.Header().Set(sessionHeader, f.session)
		}
	} else if f.expired && r.Header.Get(sessionHeader) != "" {
		http.Error(w, "gone", http.StatusNotFound)
		return
	}
	data, _ := json.Marshal(mcptest.Response(id, method, params, nil))
	if f.cutShort && method == "tools/call" {
		w.Header().Set("Content-Type", "text/event-stream")
		return
	}
	if !f.sse {
		w.Header().Set("Content-Type", "application/json")
		w.Write(data)
		return
	}
	f.writeEvents(w, data, method)
}

func (f *fakeHTTP) writeEvents(w http.ResponseWriter, data []byte, method string) {
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	fmt.Fprint(w, ": comment\n\nid: 1\ndata:\n\n")
	fmt.Fprint(w, "data: {\"jsonrpc\":\"2.0\",\"method\":\"notifications/progress\"}\n\n")
	if f.ping && method == "tools/call" {
		fmt.Fprint(w, "data: {\"jsonrpc\":\"2.0\",\"id\":\"srv-1\",\"method\":\"ping\"}\n\n")
	}
	fmt.Fprintf(w, "event: message\ndata: %s\n\n", data)
}

func httpClient(t *testing.T, f *fakeHTTP, headers map[string]string) *Client {
	t.Helper()
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	c := New(Config{Name: "web", URL: srv.URL, Headers: headers, Timeout: 2 * time.Second})
	t.Cleanup(c.Close)
	return c
}

func TestHTTPResponseShapes(t *testing.T) {
	tests := []struct {
		name string
		sse  bool
	}{
		{name: "json"},
		{name: "sse", sse: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := httpClient(t, &fakeHTTP{sse: tt.sse}, nil)
			got, err := c.Tools(context.Background())
			if err != nil || len(got) != 10 {
				t.Fatalf("tools %d %v", len(got), err)
			}
			res, err := c.Call(context.Background(), "echo", json.RawMessage(`{"a":1}`))
			if err != nil || res.Text != `echo:{"a":1}` {
				t.Fatalf("call %+v %v", res, err)
			}
		})
	}
}

func TestHTTPSendsSessionVersionAndHeaders(t *testing.T) {
	f := &fakeHTTP{session: "sess-1"}
	c := httpClient(t, f, map[string]string{"Authorization": "Bearer t"})
	if _, err := c.Call(context.Background(), "echo", nil); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	seen := append([]http.Header(nil), f.seen...)
	f.mu.Unlock()
	if len(seen) != 3 {
		t.Fatalf("requests %d", len(seen))
	}
	if seen[0].Get(sessionHeader) != "" || seen[0].Get(versionHeader) != "" {
		t.Fatalf("initialize headers %v", seen[0])
	}
	for _, h := range seen {
		if h.Get("Authorization") != "Bearer t" {
			t.Fatalf("auth missing %v", h)
		}
		if !strings.Contains(h.Get("Accept"), "text/event-stream") {
			t.Fatalf("accept %v", h)
		}
	}
	for _, h := range seen[1:] {
		if h.Get(sessionHeader) != "sess-1" || h.Get(versionHeader) != protocolVersion {
			t.Fatalf("headers %v", h)
		}
	}
}

func TestHTTPCloseEndsTheSession(t *testing.T) {
	f := &fakeHTTP{session: "sess-1"}
	c := httpClient(t, f, nil)
	if _, err := c.Call(context.Background(), "echo", nil); err != nil {
		t.Fatal(err)
	}
	c.Close()
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.deletes != 1 {
		t.Fatalf("deletes %d", f.deletes)
	}
}

func TestHTTPExpiredSessionReconnects(t *testing.T) {
	f := &fakeHTTP{session: "sess-1"}
	c := httpClient(t, f, nil)
	if _, err := c.Call(context.Background(), "echo", nil); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	f.expired = true
	f.mu.Unlock()
	_, err := c.Call(context.Background(), "echo", nil)
	if err == nil || !strings.Contains(err.Error(), "session expired") {
		t.Fatalf("err %v", err)
	}
	if _, err := c.Call(context.Background(), "echo", nil); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.inits != 2 {
		t.Fatalf("inits %d", f.inits)
	}
}

func TestHTTPAnswersServerPing(t *testing.T) {
	f := &fakeHTTP{sse: true, ping: true}
	c := httpClient(t, f, nil)
	if _, err := c.Call(context.Background(), "echo", nil); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.replies) != 1 || f.replies[0] != `"srv-1"` {
		t.Fatalf("replies %v", f.replies)
	}
}

func TestHTTPStreamWithoutResponseFails(t *testing.T) {
	c := httpClient(t, &fakeHTTP{cutShort: true}, nil)
	_, err := c.Call(context.Background(), "echo", nil)
	if err == nil || !strings.Contains(err.Error(), "without a response") {
		t.Fatalf("err %v", err)
	}
}

func TestHTTPStatusErrors(t *testing.T) {
	tests := []struct {
		name   string
		status int
		want   string
	}{
		{name: "unauthorized", status: http.StatusUnauthorized, want: "http 401: denied"},
		{name: "server error", status: http.StatusBadGateway, want: "http 502: denied"},
		{name: "redirect is not followed", status: http.StatusFound, want: "http 302"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Location", "http://127.0.0.1:1/elsewhere")
				http.Error(w, "denied", tt.status)
			}))
			defer srv.Close()
			c := New(Config{Name: "web", URL: srv.URL, Timeout: time.Second})
			defer c.Close()
			_, err := c.Tools(context.Background())
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err %v", err)
			}
		})
	}
}

func TestHTTPTimeout(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
	}))
	defer srv.Close()
	defer close(release)
	c := New(Config{Name: "web", URL: srv.URL, Timeout: 100 * time.Millisecond})
	defer c.Close()
	if _, err := c.Tools(context.Background()); err == nil {
		t.Fatal("want timeout")
	}
}

func TestHTTPOutputIsCapped(t *testing.T) {
	c := httpClient(t, &fakeHTTP{sse: true}, nil)
	res, err := c.Call(context.Background(), "big", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Text) > tool.MaxOutput {
		t.Fatalf("len %d", len(res.Text))
	}
}

func TestDecodeBodyRejectsOversizedAndMismatched(t *testing.T) {
	tests := []struct {
		name, body, want string
	}{
		{name: "oversized", body: strings.Repeat("x", maxMessage+1), want: "over"},
		{name: "wrong id", body: `{"id":9,"result":{}}`, want: "does not match"},
		{name: "empty", body: " ", want: "without a response"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := decodeBody(strings.NewReader(tt.body), 1)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err %v", err)
			}
		})
	}
}

func TestUnsupportedProtocolIsRefused(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":"1999-01-01"}}`)
	}))
	defer srv.Close()
	c := New(Config{Name: "web", URL: srv.URL, Timeout: time.Second})
	defer c.Close()
	_, err := c.Tools(context.Background())
	if err == nil || !strings.Contains(err.Error(), "unsupported protocol") {
		t.Fatalf("err %v", err)
	}
}

func TestContentSummaries(t *testing.T) {
	tests := []struct {
		name, raw, want string
	}{
		{name: "audio", raw: `{"content":[{"type":"audio","mimeType":"audio/wav","data":"QUJD"}]}`, want: "[audio audio/wav, 3 bytes]"},
		{name: "resource text", raw: `{"content":[{"type":"resource","resource":{"uri":"a://b","text":"body"}}]}`, want: "body"},
		{name: "resource blob", raw: `{"content":[{"type":"resource","resource":{"uri":"a://b","mimeType":"x/y","blob":"QUJD"}}]}`, want: "[resource a://b x/y, 3 bytes]"},
		{name: "resource link", raw: `{"content":[{"type":"resource_link","uri":"a://b"}]}`, want: "[resource link a://b]"},
		{name: "unknown", raw: `{"content":[{"type":"hologram"}]}`, want: "[hologram content]"},
		{name: "structured only", raw: `{"content":[],"structuredContent":{"a":1}}`, want: `{"a":1}`},
		{name: "empty", raw: `{"content":[]}`, want: emptyOutput},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := decodeCall(json.RawMessage(tt.raw))
			if err != nil || res.Text != tt.want {
				t.Fatalf("got %q %v", res.Text, err)
			}
		})
	}
}
