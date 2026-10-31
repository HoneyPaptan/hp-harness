package jev

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func spendQuestions(n int) map[string]Question {
	qs := make(map[string]Question, n)
	for i := 0; i < n; i++ {
		qs[questionName(i)] = Noul("q")
	}
	return qs
}

func spendServer(t *testing.T, status func(hit int64, qs map[string]wireQuestion) int) *httptest.Server {
	t.Helper()
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req wireRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
		}
		if code := status(hits.Add(1), req.Questions); code != http.StatusOK {
			w.WriteHeader(code)
			return
		}
		answers := make([]string, 0, len(req.Questions))
		for name := range req.Questions {
			answers = append(answers, fmt.Sprintf(`%q:{"type":"noul","noul":0.5}`, name))
		}
		fmt.Fprintf(w, `{"answers":{%s},"usage":{"input_tokens":10,"output_tokens":2}}`, strings.Join(answers, ","))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestSpent(t *testing.T) {
	ok := func(int64, map[string]wireQuestion) int { return http.StatusOK }
	tests := []struct {
		name    string
		qs      int
		status  func(int64, map[string]wireQuestion) int
		wantErr bool
		want    Spend
	}{
		{name: "one", qs: 1, status: ok, want: Spend{Requests: 1, InputTokens: 10, OutputTokens: 2}},
		{name: "split", qs: 300, status: ok, want: Spend{Requests: 3, InputTokens: 30, OutputTokens: 6}},
		{
			name: "retry counts both requests", qs: 1,
			status: func(hit int64, _ map[string]wireQuestion) int {
				if hit == 1 {
					return http.StatusTooManyRequests
				}
				return http.StatusOK
			},
			want: Spend{Requests: 2, InputTokens: 10, OutputTokens: 2},
		},
		{
			name: "failed ask counts the request, no tokens", qs: 1, wantErr: true,
			status: func(int64, map[string]wireQuestion) int { return http.StatusBadRequest },
			want:   Spend{Requests: 1},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := spendServer(t, tt.status)
			c := Client{URL: srv.URL, Key: "k", Model: DefaultModel, Sleep: func(time.Duration) {}}
			_, _, err := c.Ask(context.Background(), nil, spendQuestions(tt.qs))
			if (err != nil) != tt.wantErr {
				t.Fatalf("err %v", err)
			}
			if got := c.Spent(); got != tt.want {
				t.Fatalf("spent %+v want %+v", got, tt.want)
			}
		})
	}
}

func TestSpentKeepsEarlierAsksAfterFailure(t *testing.T) {
	srv := spendServer(t, func(hit int64, _ map[string]wireQuestion) int {
		if hit > 3 {
			return http.StatusBadRequest
		}
		return http.StatusOK
	})
	c := Client{URL: srv.URL, Key: "k", Model: DefaultModel}
	if _, _, err := c.Ask(context.Background(), nil, spendQuestions(300)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := c.Ask(context.Background(), nil, spendQuestions(1)); err == nil {
		t.Fatal("want error")
	}
	want := Spend{Requests: 4, InputTokens: 30, OutputTokens: 6}
	if got := c.Spent(); got != want {
		t.Fatalf("spent %+v want %+v", got, want)
	}
}

func TestSpentFailedChunkKeepsOnlyFinishedChunks(t *testing.T) {
	srv := spendServer(t, func(_ int64, qs map[string]wireQuestion) int {
		if _, ok := qs[questionName(0)]; ok {
			return http.StatusBadRequest
		}
		return http.StatusOK
	})
	c := Client{URL: srv.URL, Key: "k", Model: DefaultModel}
	if _, _, err := c.Ask(context.Background(), nil, spendQuestions(300)); err == nil {
		t.Fatal("want error")
	}
	got := c.Spent()
	if got.Requests < 1 || got.Requests > 3 {
		t.Fatalf("requests %d", got.Requests)
	}
	if got.InputTokens%10 != 0 || got.InputTokens > 20 {
		t.Fatalf("input tokens %d", got.InputTokens)
	}
	if got.OutputTokens != got.InputTokens/5 {
		t.Fatalf("spent %+v", got)
	}
}

func TestSpentConcurrentAsks(t *testing.T) {
	srv := spendServer(t, func(int64, map[string]wireQuestion) int { return http.StatusOK })
	c := Client{URL: srv.URL, Key: "k", Model: DefaultModel}
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, _, err := c.Ask(context.Background(), nil, spendQuestions(300)); err != nil {
				t.Error(err)
			}
			c.Spent()
		}()
	}
	wg.Wait()
	want := Spend{Requests: 12, InputTokens: 120, OutputTokens: 24}
	if got := c.Spent(); got != want {
		t.Fatalf("spent %+v want %+v", got, want)
	}
}
