package anthropic

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/HoneyPaptan/hp-harness/internal/provider"
)

func TestListModels(t *testing.T) {
	t.Run("one page", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/models" {
				t.Fatalf("Path = %q", r.URL.Path)
			}
			if got := r.URL.Query().Get("limit"); got != "1000" {
				t.Fatalf("limit = %q", got)
			}
			if got := r.Header.Get("x-api-key"); got != "anthropic-key" {
				t.Fatalf("x-api-key = %q", got)
			}
			if got := r.Header.Get("anthropic-version"); got != apiVersion {
				t.Fatalf("anthropic-version = %q", got)
			}
			if got := r.URL.Query().Get("after_id"); got != "" {
				t.Fatalf("after_id = %q want empty", got)
			}
			io.WriteString(w, `{"data":[{"id":"claude-3"},{"id":"claude-3"}],"has_more":false,"last_id":""}`)
		}))
		defer srv.Close()
		c := &Client{BaseURL: srv.URL, APIKey: "anthropic-key", HTTP: srv.Client()}
		got, err := c.ListModels(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		want := []string{"claude-3"}
		if len(got) != len(want) || got[0] != want[0] {
			t.Fatalf("got %v want %v", got, want)
		}
	})

	t.Run("two pages", func(t *testing.T) {
		var calls atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			n := calls.Add(1)
			switch n {
			case 1:
				if got := r.URL.Query().Get("after_id"); got != "" {
					t.Fatalf("first after_id = %q", got)
				}
				io.WriteString(w, `{"data":[{"id":"a"}],"has_more":true,"last_id":"page1"}`)
			case 2:
				if got := r.URL.Query().Get("after_id"); got != "page1" {
					t.Fatalf("second after_id = %q want page1", got)
				}
				io.WriteString(w, `{"data":[{"id":"b"}],"has_more":false,"last_id":""}`)
			default:
				t.Fatalf("unexpected request %d", n)
			}
		}))
		defer srv.Close()
		c := &Client{BaseURL: srv.URL, APIKey: "k", HTTP: srv.Client()}
		got, err := c.ListModels(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		want := []string{"a", "b"}
		if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
			t.Fatalf("got %v want %v", got, want)
		}
		if calls.Load() != 2 {
			t.Fatalf("calls = %d want 2", calls.Load())
		}
	})

	t.Run("401 error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		}))
		defer srv.Close()
		c := &Client{BaseURL: srv.URL, APIKey: "k", HTTP: srv.Client()}
		_, err := c.ListModels(context.Background())
		if err == nil {
			t.Fatal("expected error")
		}
		var he *provider.HTTPError
		if !errors.As(err, &he) || he.Status != http.StatusUnauthorized {
			t.Fatalf("err = %v", err)
		}
	})
}
