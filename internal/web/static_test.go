package web

import (
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

const staticBudget = 250 * 1024

var (
	inlineScript  = regexp.MustCompile(`(?is)<script[^>]*>\s*[^<\s]`)
	inlineHandler = regexp.MustCompile(`(?i)<[^>]+\son[a-z]+=`)
	remoteURL     = regexp.MustCompile(`(?i)https?://`)
)

func TestStaticContentTypes(t *testing.T) {
	cases := []struct {
		path string
		want string
	}{
		{"/", "text/html; charset=utf-8"},
		{"/index.html", "text/html; charset=utf-8"},
		{"/app.js", "text/javascript; charset=utf-8"},
		{"/rail.js", "text/javascript; charset=utf-8"},
		{"/transcript.js", "text/javascript; charset=utf-8"},
		{"/app.css", "text/css; charset=utf-8"},
		{"/base.css", "text/css; charset=utf-8"},
		{"/tokens.css", "text/css; charset=utf-8"},
		{"/fonts/Geist-Regular.woff2", "font/woff2"},
		{"/fonts/Geist-Medium.woff2", "font/woff2"},
		{"/fonts/GeistMono-Variable.woff2", "font/woff2"},
	}
	h := newHarness(t, newBackend(nil), nil)
	for _, c := range cases {
		t.Run(c.path, func(t *testing.T) {
			got := getStatic(t, h, c.path)
			if got != c.want {
				t.Fatalf("Content-Type = %q, want %q", got, c.want)
			}
		})
	}
}

func TestEveryStaticFileHasAKnownType(t *testing.T) {
	files, err := loadStatic()
	if err != nil {
		t.Fatal(err)
	}
	for name := range files {
		if staticType(name) == "" {
			t.Errorf("%s has no content type", name)
		}
	}
}

func TestStaticFilesStayInert(t *testing.T) {
	files, err := loadStatic()
	if err != nil {
		t.Fatal(err)
	}
	checks := []struct {
		name  string
		found func(string) bool
	}{
		{"innerHTML", func(s string) bool { return strings.Contains(s, "innerHTML") }},
		{"style attribute", func(s string) bool { return strings.Contains(s, "style=") }},
		{"inline script", inlineScript.MatchString},
		{"inline handler", inlineHandler.MatchString},
		{"remote URL", remoteURL.MatchString},
	}
	for name, f := range files {
		for _, c := range checks {
			if c.found(string(f.data)) {
				t.Errorf("%s contains %s", name, c.name)
			}
		}
	}
}

func TestStaticSizeBudget(t *testing.T) {
	files, err := loadStatic()
	if err != nil {
		t.Fatal(err)
	}
	total := 0
	for _, f := range files {
		total += len(f.data)
	}
	if total > staticBudget {
		t.Fatalf("static files are %d bytes, budget %d", total, staticBudget)
	}
}

func getStatic(t *testing.T, h *harness, path string) string {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Host = testHost
	req.Header.Set("Authorization", bearerPrefix+testToken)
	h.srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("%s: status %d", path, rec.Code)
	}
	if body, _ := io.ReadAll(rec.Body); len(body) == 0 {
		t.Fatalf("%s: empty body", path)
	}
	return rec.Header().Get("Content-Type")
}
