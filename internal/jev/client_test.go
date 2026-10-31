package jev

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestAskRequestGolden(t *testing.T) {
	var gotAuth string
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"jev-latest","answers":{"risk":{"type":"noul","noul":0.83}},"usage":{"input_tokens":10,"output_tokens":5}}`))
	}))
	defer srv.Close()

	c := Client{URL: srv.URL, Key: "secret", Model: DefaultModel}
	state := map[string]int{"x": 1}
	qs := map[string]Question{
		"risk": Noul("rate risk"),
	}
	ans, use, err := c.Ask(context.Background(), state, qs)
	if err != nil {
		t.Fatal(err)
	}
	if gotAuth != "Bearer secret" {
		t.Fatalf("auth %q", gotAuth)
	}
	var req wireRequest
	err = json.Unmarshal(gotBody, &req)
	if err != nil {
		t.Fatal(err)
	}
	if req.Model != DefaultModel {
		t.Fatalf("model %q", req.Model)
	}
	var st map[string]int
	if err := reencode(req.State, &st); err != nil || st["x"] != 1 {
		t.Fatalf("state %#v", req.State)
	}
	rq, ok := req.Questions["risk"]
	if !ok || rq.Type != "noul" || rq.Instructions != "rate risk" {
		t.Fatalf("questions %#v", req.Questions)
	}
	if ans["risk"].Type != "noul" || ans["risk"].Noul != 0.83 {
		t.Fatalf("answer %#v", ans["risk"])
	}
	if use.InputTokens != 10 || use.OutputTokens != 5 {
		t.Fatalf("usage %#v", use)
	}
}

func TestAskChoiceParse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.ReadAll(r.Body)
		_, _ = w.Write([]byte(`{"model":"m","answers":{"pick":{"type":"choice","choice":"a","confidence":0.9,"probabilities":{"a":0.9,"b":0.1}}},"usage":{"input_tokens":1,"output_tokens":2}}`))
	}))
	defer srv.Close()

	c := Client{URL: srv.URL, Key: "k", Model: "m"}
	ans, use, err := c.Ask(context.Background(), nil, map[string]Question{
		"pick": Choice("choose", map[string]string{"a": "A", "b": "B"}),
	})
	if err != nil {
		t.Fatal(err)
	}
	a := ans["pick"]
	if a.Type != "choice" || a.Choice != "a" || a.Confidence != 0.9 {
		t.Fatalf("answer %#v", a)
	}
	if use.InputTokens != 1 || use.OutputTokens != 2 {
		t.Fatalf("usage %#v", use)
	}
}

func TestAskUsageSummedAcrossChunks(t *testing.T) {
	var n atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n.Add(1)
		body, _ := io.ReadAll(r.Body)
		var req wireRequest
		_ = json.Unmarshal(body, &req)
		answers := make(map[string]wireAnswer)
		for name := range req.Questions {
			answers[name] = wireAnswer{Type: "noul", Noul: 0.5}
		}
		out, _ := json.Marshal(wireResponse{
			Model:   req.Model,
			Answers: answers,
			Usage:   wireUsage{InputTokens: 3, OutputTokens: 4},
		})
		_, _ = w.Write(out)
	}))
	defer srv.Close()

	qs := make(map[string]Question, 130)
	for i := range 130 {
		qs[padName(i)] = Noul(i)
	}

	c := Client{URL: srv.URL, Key: "k", Model: "m"}
	_, use, err := c.Ask(context.Background(), nil, qs)
	if err != nil {
		t.Fatal(err)
	}
	if n.Load() != 2 {
		t.Fatalf("requests %d", n.Load())
	}
	if use.InputTokens != 6 || use.OutputTokens != 8 {
		t.Fatalf("usage %#v", use)
	}
}

func padName(i int) string {
	return fmt.Sprintf("n%04d", i)
}

func TestAsk300Questions(t *testing.T) {
	var n atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n.Add(1)
		body, _ := io.ReadAll(r.Body)
		var req wireRequest
		_ = json.Unmarshal(body, &req)
		answers := make(map[string]wireAnswer, len(req.Questions))
		for name := range req.Questions {
			answers[name] = wireAnswer{Type: "noul", Noul: 1}
		}
		out, _ := json.Marshal(wireResponse{
			Model:   req.Model,
			Answers: answers,
			Usage:   wireUsage{InputTokens: 1, OutputTokens: 1},
		})
		_, _ = w.Write(out)
	}))
	defer srv.Close()

	qs := make(map[string]Question, 300)
	for i := range 300 {
		qs[padName(i)] = Noul(i)
	}

	c := Client{URL: srv.URL, Key: "k", Model: "m"}
	ans, _, err := c.Ask(context.Background(), nil, qs)
	if err != nil {
		t.Fatal(err)
	}
	if n.Load() != 3 {
		t.Fatalf("requests %d", n.Load())
	}
	if len(ans) != 300 {
		t.Fatalf("answers %d", len(ans))
	}
}

func TestPostRetry429Then200(t *testing.T) {
	var n atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if n.Add(1) == 1 {
			w.WriteHeader(429)
			_, _ = w.Write([]byte("slow down"))
			return
		}
		_, _ = io.ReadAll(r.Body)
		_, _ = w.Write([]byte(`{"model":"m","answers":{},"usage":{"input_tokens":0,"output_tokens":0}}`))
	}))
	defer srv.Close()

	c := Client{URL: srv.URL, Key: "k", Model: "m", Sleep: func(time.Duration) {}}
	_, _, err := c.Ask(context.Background(), nil, map[string]Question{"a": Noul(1)})
	if err != nil {
		t.Fatal(err)
	}
	if n.Load() != 2 {
		t.Fatalf("requests %d", n.Load())
	}
}

func TestPost400Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		_, _ = w.Write([]byte("bad request detail"))
	}))
	defer srv.Close()

	c := Client{URL: srv.URL, Key: "k", Model: "m", Sleep: func(time.Duration) {}}
	_, _, err := c.Ask(context.Background(), nil, map[string]Question{"a": Noul(1)})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "400") || !strings.Contains(err.Error(), "bad request") {
		t.Fatalf("err %q", err)
	}
}

func TestFromEnv(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		wantURL string
		wantKey string
		wantMod string
		wantErr error
	}{
		{
			name:    "typesafe wins",
			env:     map[string]string{"TYPESAFE_API_KEY": "ts", "OPENROUTER_API_KEY": "or"},
			wantURL: TypeSafeURL,
			wantKey: "ts",
			wantMod: DefaultModel,
		},
		{
			name:    "openrouter fallback",
			env:     map[string]string{"OPENROUTER_API_KEY": "or"},
			wantURL: OpenRouterURL,
			wantKey: "or",
			wantMod: DefaultModel,
		},
		{
			name:    "model override",
			env:     map[string]string{"TYPESAFE_API_KEY": "ts", "HP_JEV_MODEL": "custom"},
			wantURL: TypeSafeURL,
			wantKey: "ts",
			wantMod: "custom",
		},
		{
			name:    "no key",
			env:     map[string]string{},
			wantErr: ErrNoKey,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			getenv := func(k string) string { return tt.env[k] }
			c, err := FromEnv(getenv)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("err %v", err)
				}
				if !strings.Contains(err.Error(), "TYPESAFE_API_KEY") || !strings.Contains(err.Error(), "OPENROUTER_API_KEY") {
					t.Fatalf("err message %q", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if c.URL != tt.wantURL || c.Key != tt.wantKey || c.Model != tt.wantMod {
				t.Fatalf("client %#v", c)
			}
		})
	}
}

func reencode(v any, dst any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, dst)
}

func TestPostRetriesThenFails(t *testing.T) {
	cases := []struct {
		status   int
		requests int32
	}{
		{429, 3},
		{529, 3},
		{500, 1},
	}
	for _, tc := range cases {
		var n atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			n.Add(1)
			w.WriteHeader(tc.status)
		}))
		c := Client{URL: srv.URL, Key: "k", Model: "m", Sleep: func(time.Duration) {}}
		_, _, err := c.Ask(context.Background(), nil, map[string]Question{"a": Noul(1)})
		srv.Close()
		if err == nil {
			t.Fatalf("status %d: expected error", tc.status)
		}
		if n.Load() != tc.requests {
			t.Fatalf("status %d: requests %d", tc.status, n.Load())
		}
	}
}

func TestAskChunkFailureKeepsCause(t *testing.T) {
	var n atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.ReadAll(r.Body)
		if n.Add(1) == 1 {
			w.WriteHeader(400)
			_, _ = w.Write([]byte("bad chunk"))
			return
		}
		<-r.Context().Done()
	}))
	defer srv.Close()

	qs := make(map[string]Question, 300)
	for i := range 300 {
		qs[padName(i)] = Noul(i)
	}
	c := Client{URL: srv.URL, Key: "k", Model: "m"}
	_, _, err := c.Ask(context.Background(), nil, qs)
	if err == nil || !strings.Contains(err.Error(), "bad chunk") {
		t.Fatalf("err %v", err)
	}
}

func TestPostBackoffStopsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cancel()
		w.WriteHeader(429)
	}))
	defer srv.Close()

	c := Client{URL: srv.URL, Key: "k", Model: "m"}
	start := time.Now()
	_, _, err := c.Ask(ctx, nil, map[string]Question{"a": Noul(1)})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err %v", err)
	}
	if time.Since(start) > 500*time.Millisecond {
		t.Fatalf("took %v", time.Since(start))
	}
}
