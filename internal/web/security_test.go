package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type secRow struct {
	name    string
	method  string
	target  string
	host    string
	headers map[string]string
	body    string
	want    int
}

func authJSON(extra map[string]string) map[string]string {
	h := map[string]string{"Authorization": "Bearer " + testToken, "Content-Type": "application/json"}
	for k, v := range extra {
		h[k] = v
	}
	return h
}

func securityRows() []secRow {
	good := `{"prompt":"hi"}`
	huge := `{"prompt":"` + strings.Repeat("x", maxBody) + `"}`
	return []secRow{
		{name: "no token", method: "POST", target: "/api/runs", headers: map[string]string{"Content-Type": "application/json"}, body: good, want: 401},
		{name: "no token get", method: "GET", target: "/api/state", want: 401},
		{name: "no token page", method: "GET", target: "/", want: 401},
		{name: "bad bearer", method: "POST", target: "/api/runs", headers: authJSON(map[string]string{"Authorization": "Bearer nope"}), body: good, want: 401},
		{name: "basic scheme", method: "GET", target: "/api/state", headers: map[string]string{"Authorization": "Basic " + testToken}, want: 401},
		{name: "bad cookie", method: "GET", target: "/api/state", headers: map[string]string{"Cookie": "hp_token_7878=nope"}, want: 401},
		{name: "bad query token", method: "GET", target: "/?token=nope", want: 401},
		{name: "bad query token with bearer", method: "GET", target: "/api/state?token=nope", headers: authJSON(nil), want: 401},
		{name: "query token on post", method: "POST", target: "/api/runs?token=" + testToken, headers: map[string]string{"Content-Type": "application/json"}, body: good, want: 401},
		{name: "bad host", method: "POST", target: "/api/runs", host: "evil.example", headers: authJSON(nil), body: good, want: 403},
		{name: "bad host right port", method: "GET", target: "/api/state", host: "evil.example:7878", headers: authJSON(nil), want: 403},
		{name: "wrong port", method: "GET", target: "/api/state", host: "127.0.0.1:1", headers: authJSON(nil), want: 403},
		{name: "foreign origin", method: "POST", target: "/api/runs", headers: authJSON(map[string]string{"Origin": "http://evil.example"}), body: good, want: 403},
		{name: "null origin", method: "POST", target: "/api/runs", headers: authJSON(map[string]string{"Origin": "null"}), body: good, want: 403},
		{name: "missing content type", method: "POST", target: "/api/runs", headers: map[string]string{"Authorization": "Bearer " + testToken}, body: good, want: 415},
		{name: "form content type", method: "POST", target: "/api/runs", headers: authJSON(map[string]string{"Content-Type": "text/plain"}), body: good, want: 415},
		{name: "oversized body", method: "POST", target: "/api/runs", headers: authJSON(nil), body: huge, want: 413},
		{name: "empty prompt", method: "POST", target: "/api/runs", headers: authJSON(nil), body: `{"prompt":"  "}`, want: 400},
		{name: "bad json", method: "POST", target: "/api/runs", headers: authJSON(nil), body: `{`, want: 400},
		{name: "bad model", method: "POST", target: "/api/runs", headers: authJSON(nil), body: `{"prompt":"x","model":"bad/model"}`, want: 400},
		{name: "traversal events", method: "GET", target: "/api/runs/..%2f..%2fetc%2fpasswd/events", headers: authJSON(nil), want: 400},
		{name: "traversal message", method: "POST", target: "/api/runs/..%2fsecret/messages", headers: authJSON(nil), body: good, want: 400},
		{name: "dotted id", method: "POST", target: "/api/runs/20260101-120000-00.1/stop", headers: authJSON(nil), body: "{}", want: 400},
		{name: "unknown run", method: "GET", target: "/api/runs/20260101-120000-ffff/events", headers: authJSON(nil), want: 404},
	}
}

func checkHeaders(t *testing.T, h http.Header) {
	t.Helper()
	if h.Get("Content-Security-Policy") != cspPolicy {
		t.Fatalf("csp %q", h.Get("Content-Security-Policy"))
	}
	if h.Get("X-Content-Type-Options") != "nosniff" || h.Get("Referrer-Policy") != "no-referrer" {
		t.Fatalf("headers %v", h)
	}
	for key := range h {
		if strings.HasPrefix(key, "Access-Control-") {
			t.Fatalf("cors header %s", key)
		}
	}
}

func TestSecurity(t *testing.T) {
	for _, row := range securityRows() {
		t.Run(row.name, func(t *testing.T) {
			b := newBackend(&scripted{})
			b.h.Append("/secret.jsonl", []byte("{}\n"))
			srv, err := New(Options{Backend: b, Token: testToken, Hosts: AllowedHosts(testHost)})
			if err != nil {
				t.Fatal(err)
			}
			defer srv.Close()
			req := httptest.NewRequest(row.method, row.target, strings.NewReader(row.body))
			req.Host = testHost
			if row.host != "" {
				req.Host = row.host
			}
			for k, v := range row.headers {
				req.Header.Set(k, v)
			}
			rec := httptest.NewRecorder()
			srv.Handler().ServeHTTP(rec, req)
			if rec.Code != row.want {
				t.Fatalf("code %d want %d: %s", rec.Code, row.want, rec.Body.String())
			}
			checkHeaders(t, rec.Header())
			if b.startCount() != 0 {
				t.Fatal("a run started")
			}
		})
	}
}

func TestTokenExchange(t *testing.T) {
	srv, err := New(Options{Backend: newBackend(&scripted{}), Token: testToken, Hosts: AllowedHosts(testHost)})
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	h := srv.Handler()
	req := httptest.NewRequest("GET", "//evil.example/?token="+testToken, nil)
	req.Host = testHost
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/" {
		t.Fatalf("%d %q", rec.Code, rec.Header().Get("Location"))
	}
	checkHeaders(t, rec.Header())
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies %v", cookies)
	}
	c := cookies[0]
	if !c.HttpOnly || c.SameSite != http.SameSiteStrictMode || c.Path != "/" {
		t.Fatalf("cookie %+v", c)
	}
	req = httptest.NewRequest("GET", "/api/state", nil)
	req.Host = "localhost:7878"
	req.AddCookie(c)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("cookie refused: %d", rec.Code)
	}
}

func TestSameOriginAccepted(t *testing.T) {
	b := newBackend(&scripted{turns: [][]step{{{text: "ok"}}}})
	srv, err := New(Options{Backend: b, Token: testToken, Hosts: AllowedHosts(testHost)})
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	req := httptest.NewRequest("POST", "/api/runs", strings.NewReader(`{"prompt":"hi"}`))
	req.Host = testHost
	req.Header.Set("Authorization", "Bearer "+testToken)
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Origin", "http://"+testHost)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
}

func TestNewToken(t *testing.T) {
	a, err := NewToken(nil)
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewToken(nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(a) != 64 || a == b {
		t.Fatalf("%q %q", a, b)
	}
}

func TestNewRequiresToken(t *testing.T) {
	if _, err := New(Options{Backend: newBackend(&scripted{})}); err == nil {
		t.Fatal("no error")
	}
}

func TestStaticEtag(t *testing.T) {
	srv, err := New(Options{Backend: newBackend(&scripted{}), Token: testToken, Hosts: AllowedHosts(testHost)})
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	get := func(target, etag string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("GET", target, nil)
		req.Host = testHost
		req.Header.Set("Authorization", "Bearer "+testToken)
		if etag != "" {
			req.Header.Set("If-None-Match", etag)
		}
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		return rec
	}
	first := get("/", "")
	etag := first.Header().Get("ETag")
	if first.Code != 200 || etag == "" || !strings.HasPrefix(first.Header().Get("Content-Type"), "text/html") {
		t.Fatalf("%d %v", first.Code, first.Header())
	}
	if got := get("/", etag); got.Code != http.StatusNotModified {
		t.Fatalf("code %d", got.Code)
	}
	if got := get("/nothing.js", ""); got.Code != 404 {
		t.Fatalf("code %d", got.Code)
	}
}

func TestValidRunID(t *testing.T) {
	cases := map[string]bool{
		"20260101-120000-0a1f":  true,
		"20260101-120000-0A1F":  false,
		"../20260101-120000-00": false,
		"20260101-120000-0a1f/": false,
		"":                      false,
	}
	for id, want := range cases {
		if ValidRunID(id) != want {
			t.Fatalf("%q", id)
		}
	}
}
