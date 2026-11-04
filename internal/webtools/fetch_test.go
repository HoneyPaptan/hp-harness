package webtools

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/HoneyPaptan/hp-harness/internal/tool"
)

func TestFetchHTMLGolden(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		io.WriteString(w, sampleHTML)
	}))
	t.Cleanup(srv.Close)

	client := NewClient(true)
	ft := FetchTool(client)
	res, err := ft.Run(context.Background(), tool.Env{}, mustArgs(t, srv.URL))
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Text)
	}
	wantPrefix := srv.URL + "\nGolden Title\n\n" + wantGoldenBody
	if res.Text != wantPrefix {
		t.Fatalf("output mismatch:\n%s", diffLines(res.Text, wantPrefix))
	}
}

func TestFetchPlainJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/plain":
			w.Header().Set("Content-Type", "text/plain")
			io.WriteString(w, "hello plain")
		case "/json":
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"a":1}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	client := NewClient(true)
	ft := FetchTool(client)
	ctx := context.Background()
	env := tool.Env{}

	res, err := ft.Run(ctx, env, mustArgs(t, srv.URL+"/plain"))
	if err != nil || res.IsError {
		t.Fatalf("plain: err=%v res=%+v", err, res)
	}
	if !strings.HasSuffix(res.Text, "hello plain") {
		t.Fatalf("plain body: %q", res.Text)
	}

	res, err = ft.Run(ctx, env, mustArgs(t, srv.URL+"/json"))
	if err != nil || res.IsError {
		t.Fatalf("json: err=%v res=%+v", err, res)
	}
	if !strings.HasSuffix(res.Text, `{"a":1}`) {
		t.Fatalf("json body: %q", res.Text)
	}
}

func TestFetchUnsupportedAnd404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/png":
			w.Header().Set("Content-Type", "image/png")
			w.Write([]byte{1, 2, 3})
		case "/missing":
			http.Error(w, "gone", http.StatusNotFound)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	client := NewClient(true)
	ft := FetchTool(client)
	ctx := context.Background()
	env := tool.Env{}

	res, err := ft.Run(ctx, env, mustArgs(t, srv.URL+"/png"))
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError || !strings.Contains(res.Text, "unsupported content type image/png") {
		t.Fatalf("png: %+v", res)
	}

	res, err = ft.Run(ctx, env, mustArgs(t, srv.URL+"/missing"))
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError || !strings.HasPrefix(res.Text, "http 404") {
		t.Fatalf("404: %+v", res)
	}
}

func TestFetchOutputTruncation(t *testing.T) {
	payload := strings.Repeat("x", 70*1024)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		io.WriteString(w, payload)
	}))
	t.Cleanup(srv.Close)

	client := NewClient(true)
	ft := FetchTool(client)
	res, err := ft.Run(context.Background(), tool.Env{}, mustArgs(t, srv.URL))
	if err != nil || res.IsError {
		t.Fatalf("err=%v res=%+v", err, res)
	}
	if !strings.HasSuffix(res.Text, truncationLine) {
		t.Fatalf("missing truncation marker, len=%d", len(res.Text))
	}
	if len(res.Text) > maxOutputBytes+len(truncationLine)+2 {
		t.Fatalf("output too long: %d", len(res.Text))
	}
}

func TestFetchReadLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		payload := strings.Repeat("z", 1024)
		for i := 0; i < 3*1024; i++ {
			if _, err := w.Write([]byte(payload)); err != nil {
				return
			}
		}
	}))
	t.Cleanup(srv.Close)

	client := NewClient(true)
	req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := readLimited(resp.Body, maxReadBody)
	if err != nil {
		t.Fatal(err)
	}
	if len(body) != maxReadBody {
		t.Fatalf("read %d bytes, want %d", len(body), maxReadBody)
	}
}

func TestFetchGuardedLocalhost(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "ok")
	}))
	t.Cleanup(srv.Close)

	client := NewClient(false)
	ft := FetchTool(client)
	res, err := ft.Run(context.Background(), tool.Env{}, mustArgs(t, srv.URL))
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError || !strings.Contains(res.Text, refusedPrivate) {
		t.Fatalf("got %+v", res)
	}
}

func TestValidateFetchURL(t *testing.T) {
	bad := []string{
		"ftp://example.com",
		"http://",
		"https://u:p@example.com",
		"",
	}
	for _, u := range bad {
		if _, err := validateFetchURL(u); err == nil {
			t.Fatalf("expected error for %q", u)
		}
	}
	if u, err := validateFetchURL("https://example.com/path"); err != nil || u.Host != "example.com" {
		t.Fatalf("valid url: u=%v err=%v", u, err)
	}
}

func mustArgs(t *testing.T, url string) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(map[string]string{"url": url})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestFetchConnectFailureIsToolError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	addr := srv.URL
	srv.Close()
	res, err := FetchTool(NewClient(true)).Run(context.Background(), tool.Env{}, mustArgs(t, addr))
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError || !strings.Contains(res.Text, "fetch failed") {
		t.Fatalf("got %+v", res)
	}
}

func TestFetchCanceledIsGoError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := FetchTool(NewClient(true)).Run(ctx, tool.Env{}, mustArgs(t, "http://127.0.0.1:1"))
	if err == nil {
		t.Fatal("want error")
	}
}

func TestBlockedIPRanges(t *testing.T) {
	for _, s := range []string{"127.0.0.1", "10.1.2.3", "172.20.0.1", "192.168.0.5", "169.254.1.1", "100.64.0.1", "0.0.0.0", "224.0.0.1", "::1", "::", "fe80::1", "fd00::1", "ff02::1", "::ffff:127.0.0.1"} {
		if !blockedIP(net.ParseIP(s)) {
			t.Errorf("%s not blocked", s)
		}
	}
	for _, s := range []string{"8.8.8.8", "100.128.0.1", "2606:4700::1111"} {
		if blockedIP(net.ParseIP(s)) {
			t.Errorf("%s blocked", s)
		}
	}
}
