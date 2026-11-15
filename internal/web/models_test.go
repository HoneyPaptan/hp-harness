package web

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

type listingBackend struct {
	*testBackend
	models []string
	err    error
	preset string
}

func (b *listingBackend) Models(_ context.Context, preset string) ([]string, error) {
	b.preset = preset
	return b.models, b.err
}

func getModels(t *testing.T, srv *Server, query string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/models"+query, nil)
	req.Host = testHost
	req.Header.Set("Authorization", bearerPrefix+testToken)
	srv.Handler().ServeHTTP(rec, req)
	return rec
}

func modelServer(t *testing.T, b Backend) *Server {
	t.Helper()
	srv, err := New(Options{Backend: b, Token: testToken, Hosts: []string{testHost}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(srv.Close)
	return srv
}

func TestModelsListsForPreset(t *testing.T) {
	b := &listingBackend{testBackend: newBackend(nil), models: []string{"a:free", "b"}}
	srv := modelServer(t, b)
	rec := getModels(t, srv, "?preset=openrouter")
	var body modelsBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("code %d body %s", rec.Code, rec.Body)
	}
	if b.preset != "openrouter" || len(body.Models) != 2 {
		t.Fatalf("preset %q body %+v", b.preset, body)
	}
}

func TestModelsNeedsPreset(t *testing.T) {
	srv := modelServer(t, &listingBackend{testBackend: newBackend(nil)})
	if rec := getModels(t, srv, ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("code %d", rec.Code)
	}
}

func TestModelsUnsupportedBackend(t *testing.T) {
	srv := modelServer(t, newBackend(nil))
	if rec := getModels(t, srv, "?preset=x"); rec.Code != http.StatusNotImplemented {
		t.Fatalf("code %d", rec.Code)
	}
}

func TestModelsFailure(t *testing.T) {
	srv := modelServer(t, &listingBackend{testBackend: newBackend(nil), err: errors.New("down")})
	if rec := getModels(t, srv, "?preset=x"); rec.Code < 400 {
		t.Fatalf("code %d", rec.Code)
	}
}
