package web

import (
	"context"
	"net/http"
)

type ModelLister interface {
	Models(ctx context.Context, preset string) ([]string, error)
}

type modelsBody struct {
	Preset string   `json:"preset"`
	Models []string `json:"models"`
}

func (s *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	lister, ok := s.mgr.b.(ModelLister)
	if !ok {
		writeError(w, http.StatusNotImplemented, "this backend cannot list models")
		return
	}
	preset := r.URL.Query().Get("preset")
	if preset == "" {
		writeError(w, http.StatusBadRequest, "preset required")
		return
	}
	models, err := lister.Models(r.Context(), preset)
	if err != nil {
		writeFailure(w, err)
		return
	}
	writeJSON(w, http.StatusOK, modelsBody{Preset: preset, Models: models})
}
