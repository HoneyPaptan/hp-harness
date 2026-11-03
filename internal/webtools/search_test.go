package webtools

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/HoneyPaptan/hp-harness/internal/tool"
)

const testExaKey = "exa-test-key-secret"

func TestSearchExaGolden(t *testing.T) {
	var captured exaSearchRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method %s", r.Method)
		}
		if r.Header.Get("x-api-key") != testExaKey {
			t.Errorf("x-api-key header %q", r.Header.Get("x-api-key"))
		}
		if ct := r.Header.Get("Content-Type"); !strings.Contains(ct, "application/json") {
			t.Errorf("content-type %q", ct)
		}
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"results":[{"title":"First Page","url":"https://a.example/one","highlights":["alpha","beta"]},{"title":"Second","url":"https://b.example/two","highlights":["gamma"]}]}`)
	}))
	t.Cleanup(srv.Close)

	st := SearchTool(srv.Client(), srv.URL, testExaKey)
	res, err := st.Run(context.Background(), tool.Env{}, mustSearchArgs(t, "hello world", nil))
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Text)
	}
	if captured.Query != "hello world" || captured.Type != "auto" || captured.NumResults != 5 || !captured.Contents.Highlights {
		t.Fatalf("request body %+v", captured)
	}
	want := "1. First Page\nhttps://a.example/one\nalpha … beta\n\n2. Second\nhttps://b.example/two\ngamma"
	if res.Text != want {
		t.Fatalf("output mismatch:\n%s", diffLines(res.Text, want))
	}
}

func TestSearchCountDefaultAndClamp(t *testing.T) {
	var numResults int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req exaSearchRequest
		json.NewDecoder(r.Body).Decode(&req)
		numResults = req.NumResults
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"results":[]}`)
	}))
	t.Cleanup(srv.Close)

	st := SearchTool(srv.Client(), srv.URL, testExaKey)
	ctx := context.Background()
	env := tool.Env{}

	res, err := st.Run(ctx, env, mustSearchArgs(t, "q", nil))
	if err != nil || res.IsError {
		t.Fatalf("default: err=%v res=%+v", err, res)
	}
	if numResults != 5 {
		t.Fatalf("default count got %d", numResults)
	}

	zero := 0
	res, err = st.Run(ctx, env, mustSearchArgs(t, "q", &zero))
	if err != nil || res.IsError {
		t.Fatalf("zero: err=%v res=%+v", err, res)
	}
	if numResults != 1 {
		t.Fatalf("clamp zero got %d", numResults)
	}

	fifty := 50
	res, err = st.Run(ctx, env, mustSearchArgs(t, "q", &fifty))
	if err != nil || res.IsError {
		t.Fatalf("fifty: err=%v res=%+v", err, res)
	}
	if numResults != 10 {
		t.Fatalf("clamp fifty got %d", numResults)
	}
}

func TestSearchEmptyQuery(t *testing.T) {
	st := SearchTool(http.DefaultClient, "http://example.invalid", testExaKey)
	res, err := st.Run(context.Background(), tool.Env{}, json.RawMessage(`{"query":"   "}`))
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError || !strings.Contains(res.Text, "query is required") {
		t.Fatalf("got %+v", res)
	}
}

func TestSearch401KeyRedacted(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		io.WriteString(w, "invalid key "+testExaKey)
	}))
	t.Cleanup(srv.Close)

	st := SearchTool(srv.Client(), srv.URL, testExaKey)
	res, err := st.Run(context.Background(), tool.Env{}, mustSearchArgs(t, "q", nil))
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError || !strings.HasPrefix(res.Text, "exa http 401:") {
		t.Fatalf("got %+v", res)
	}
	if strings.Contains(res.Text, testExaKey) {
		t.Fatalf("key leaked in %q", res.Text)
	}
}

func TestSearchNoResults(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"results":[]}`)
	}))
	t.Cleanup(srv.Close)

	st := SearchTool(srv.Client(), srv.URL, testExaKey)
	res, err := st.Run(context.Background(), tool.Env{}, mustSearchArgs(t, "q", nil))
	if err != nil || res.IsError {
		t.Fatalf("got err=%v res=%+v", err, res)
	}
	if res.Text != "no results" {
		t.Fatalf("got %q", res.Text)
	}
}

func TestSearchBadJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{not json`)
	}))
	t.Cleanup(srv.Close)

	st := SearchTool(srv.Client(), srv.URL, testExaKey)
	res, err := st.Run(context.Background(), tool.Env{}, mustSearchArgs(t, "q", nil))
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError || !strings.Contains(res.Text, "invalid exa response") {
		t.Fatalf("got %+v", res)
	}
}

func TestSearchHighlightCut(t *testing.T) {
	long := strings.Repeat("h", 400)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		payload, _ := json.Marshal(map[string]any{
			"results": []map[string]any{
				{"title": "T", "url": "https://x.example", "highlights": []string{long}},
			},
		})
		w.Write(payload)
	}))
	t.Cleanup(srv.Close)

	st := SearchTool(srv.Client(), srv.URL, testExaKey)
	res, err := st.Run(context.Background(), tool.Env{}, mustSearchArgs(t, "q", nil))
	if err != nil || res.IsError {
		t.Fatalf("got err=%v res=%+v", err, res)
	}
	wantHL := strings.Repeat("h", maxHighlightBytes)
	if !strings.Contains(res.Text, wantHL) {
		t.Fatalf("missing trimmed highlight")
	}
	if strings.Contains(res.Text, strings.Repeat("h", maxHighlightBytes+1)) {
		t.Fatalf("highlight not cut")
	}
}

func TestSearchOutputCap(t *testing.T) {
	var results []map[string]any
	for i := 0; i < 200; i++ {
		results = append(results, map[string]any{
			"title":      strings.Repeat("t", 80),
			"url":        "https://example.com/" + strings.Repeat("p", 40),
			"highlights": []string{strings.Repeat("x", 200)},
		})
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		payload, _ := json.Marshal(map[string]any{"results": results})
		w.Write(payload)
	}))
	t.Cleanup(srv.Close)

	st := SearchTool(srv.Client(), srv.URL, testExaKey)
	res, err := st.Run(context.Background(), tool.Env{}, mustSearchArgs(t, "q", nil))
	if err != nil || res.IsError {
		t.Fatalf("got err=%v res=%+v", err, res)
	}
	if len(res.Text) != maxSearchOutputBytes {
		t.Fatalf("len %d want %d", len(res.Text), maxSearchOutputBytes)
	}
}

func mustSearchArgs(t *testing.T, query string, count *int) json.RawMessage {
	t.Helper()
	m := map[string]any{"query": query}
	if count != nil {
		m["count"] = *count
	}
	raw, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
