package openai

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/HoneyPaptan/hp-harness/internal/provider"
)

func TestListModels(t *testing.T) {
	tests := []struct {
		name      string
		baseURL   string
		key       string
		status    int
		body      string
		want      []string
		wantErr   bool
		checkAuth func(t *testing.T, r *http.Request)
		checkPath func(t *testing.T, r *http.Request)
		checkHTTP func(t *testing.T, err error)
	}{
		{
			name: "ok list",
			body: `{"data":[{"id":"z-model"},{"id":"models/gemini-a"},{"id":"b-model"},{"id":"b-model"},{"id":"  c-model  "}]}`,
			want: []string{"b-model", "c-model", "gemini-a", "z-model"},
		},
		{
			name: "auth when key set",
			key:  "secret-key",
			body: `{"data":[]}`,
			want: []string{},
			checkAuth: func(t *testing.T, r *http.Request) {
				t.Helper()
				if got := r.Header.Get("Authorization"); got != "Bearer secret-key" {
					t.Fatalf("Authorization = %q", got)
				}
			},
		},
		{
			name: "no auth when key empty",
			body: `{"data":[]}`,
			want: []string{},
			checkAuth: func(t *testing.T, r *http.Request) {
				t.Helper()
				if got := r.Header.Get("Authorization"); got != "" {
					t.Fatalf("Authorization = %q want empty", got)
				}
			},
		},
		{
			name: "request path models",
			body: `{"data":[]}`,
			want: []string{},
			checkPath: func(t *testing.T, r *http.Request) {
				t.Helper()
				if r.URL.Path != "/models" {
					t.Fatalf("Path = %q", r.URL.Path)
				}
			},
		},
		{
			name:    "401 error type",
			status:  http.StatusUnauthorized,
			body:    `{"error":"nope"}`,
			wantErr: true,
			checkHTTP: func(t *testing.T, err error) {
				t.Helper()
				var he *provider.HTTPError
				if !errors.As(err, &he) || he.Status != http.StatusUnauthorized {
					t.Fatalf("err = %v", err)
				}
			},
		},
		{
			name: "empty data",
			body: `{"data":[]}`,
			want: []string{},
		},
		{
			name:    "bad json",
			body:    `{`,
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			status := tc.status
			if status == 0 {
				status = http.StatusOK
			}
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tc.checkAuth != nil {
					tc.checkAuth(t, r)
				}
				if tc.checkPath != nil {
					tc.checkPath(t, r)
				}
				w.WriteHeader(status)
				io.WriteString(w, tc.body)
			}))
			defer srv.Close()

			baseURL := tc.baseURL
			if baseURL == "" {
				baseURL = srv.URL
			}
			if tc.name == "request path models" {
				baseURL = srv.URL + "/"
			}

			c := &Client{BaseURL: baseURL, APIKey: tc.key, HTTP: srv.Client()}
			got, err := c.ListModels(context.Background())
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				if tc.checkHTTP != nil {
					tc.checkHTTP(t, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("got %v want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("got %v want %v", got, tc.want)
				}
			}
		})
	}
}
