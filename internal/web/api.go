package web

import (
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"regexp"
	"strings"
)

const maxBody = 1 << 20

var runIDShape = regexp.MustCompile(`^[0-9]{8}-[0-9]{6}-[0-9a-f]{4}$`)

func ValidRunID(id string) bool {
	return runIDShape.MatchString(id)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func writeFailure(w http.ResponseWriter, err error) {
	writeError(w, failureStatus(err), err.Error())
}

func failureStatus(err error) int {
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return http.StatusNotFound
	case errors.Is(err, ErrInvalid):
		return http.StatusBadRequest
	case errors.Is(err, ErrBusy), errors.Is(err, errActive):
		return http.StatusConflict
	case errors.Is(err, errClosed):
		return http.StatusServiceUnavailable
	}
	return http.StatusInternalServerError
}

func runID(w http.ResponseWriter, r *http.Request) (string, bool) {
	id := r.PathValue("id")
	if !ValidRunID(id) {
		writeError(w, http.StatusBadRequest, "bad run id")
		return "", false
	}
	return id, true
}

func readJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody)).Decode(v)
	if err == nil {
		return true
	}
	var tooBig *http.MaxBytesError
	if errors.As(err, &tooBig) {
		writeError(w, http.StatusRequestEntityTooLarge, "body too large")
		return false
	}
	writeError(w, http.StatusBadRequest, "bad json")
	return false
}

type promptBody struct {
	Prompt string `json:"prompt"`
	Model  string `json:"model"`
}

func readPrompt(w http.ResponseWriter, r *http.Request) (promptBody, bool) {
	var body promptBody
	if !readJSON(w, r, &body) {
		return body, false
	}
	if strings.TrimSpace(body.Prompt) == "" {
		writeError(w, http.StatusBadRequest, "prompt required")
		return body, false
	}
	return body, true
}

func (s *Server) handleState(w http.ResponseWriter, _ *http.Request) {
	s.mgr.b.Preconnect()
	writeJSON(w, http.StatusOK, s.mgr.b.State())
}

func (s *Server) handleRuns(w http.ResponseWriter, _ *http.Request) {
	runs, err := s.mgr.b.Runs()
	if err != nil {
		writeFailure(w, err)
		return
	}
	if runs == nil {
		runs = []Run{}
	}
	writeJSON(w, http.StatusOK, runs)
}

func (s *Server) handleStart(w http.ResponseWriter, r *http.Request) {
	body, ok := readPrompt(w, r)
	if !ok {
		return
	}
	id, err := s.mgr.start(body.Model, body.Prompt)
	if err != nil {
		writeFailure(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": id})
}

func (s *Server) handleMessage(w http.ResponseWriter, r *http.Request) {
	id, ok := runID(w, r)
	if !ok {
		return
	}
	body, ok := readPrompt(w, r)
	if !ok {
		return
	}
	if err := s.mgr.send(id, body.Prompt); err != nil {
		writeFailure(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"id": id})
}

func (s *Server) handleStop(w http.ResponseWriter, r *http.Request) {
	id, ok := runID(w, r)
	if !ok {
		return
	}
	stopped, err := s.mgr.stop(id)
	if err != nil {
		writeFailure(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"stopped": stopped})
}

func (s *Server) handleModel(w http.ResponseWriter, r *http.Request) {
	id, ok := runID(w, r)
	if !ok {
		return
	}
	var body struct {
		Model string `json:"model"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	if strings.TrimSpace(body.Model) == "" {
		writeError(w, http.StatusBadRequest, "model required")
		return
	}
	model, err := s.mgr.setModel(id, body.Model)
	if err != nil {
		writeFailure(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"model": model})
}
