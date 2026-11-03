package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestStateCarriesPermission(t *testing.T) {
	h := newHarness(t, newBackend(nil), nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/state", nil)
	req.Host = testHost
	req.Header.Set("Authorization", bearerPrefix+testToken)
	h.srv.Handler().ServeHTTP(rec, req)
	var st State
	if err := json.Unmarshal(rec.Body.Bytes(), &st); err != nil {
		t.Fatal(err)
	}
	if st.Permission != "auto" {
		t.Fatalf("permission %q", st.Permission)
	}
}
