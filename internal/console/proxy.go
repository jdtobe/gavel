package console

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/jdtobe/gavel/internal/kevapi"
	"github.com/jdtobe/gavel/internal/status"
)

func (s *Server) systemone(w http.ResponseWriter, r *http.Request) {
	if !s.begin() {
		detail(w, 503, "Kev is still starting")
		return
	}
	defer s.end()
	r.Body = http.MaxBytesReader(w, r.Body, MaxBodyBytes)
	decoder := json.NewDecoder(r.Body)
	var req kevapi.Request
	if err := decoder.Decode(&req); err != nil {
		var large *http.MaxBytesError
		if errors.As(err, &large) {
			detail(w, 413, "request body exceeds 10 MB")
		} else {
			detail(w, 400, err)
		}
		return
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		detail(w, 400, "expected one JSON object")
		return
	}
	if err := kevapi.ValidateRequest(req); err != nil {
		detail(w, 400, err)
		return
	}
	req.Model = kevapi.ModelName
	body, err := json.Marshal(req)
	if err != nil {
		detail(w, 400, err)
		return
	}
	s.proxy(w, r, "/v1/systemone", body)
}
func (s *Server) models(w http.ResponseWriter, r *http.Request) {
	if !s.begin() {
		writeJSON(w, 200, map[string]any{"model": s.provider.Status().Model, "ready": false})
		return
	}
	defer s.end()
	s.proxy(w, r, "/v1/models", nil)
}
func (s *Server) proxy(w http.ResponseWriter, r *http.Request, path string, body []byte) {
	ctx, cancel := context.WithCancelCause(r.Context())
	s.mu.Lock()
	s.nextID++
	id := s.nextID
	s.cancels[id] = cancel
	if s.provider.Status().Phase != status.PhaseReady {
		cancel(errRestart)
	}
	s.mu.Unlock()
	defer func() { s.mu.Lock(); delete(s.cancels, id); s.mu.Unlock(); cancel(nil) }()
	req, err := http.NewRequestWithContext(ctx, r.Method, s.upstream()+path, bytes.NewReader(body))
	if err != nil {
		detail(w, 502, err)
		return
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := s.client.Do(req)
	if err != nil {
		if errors.Is(context.Cause(ctx), errRestart) {
			detail(w, 503, errRestart)
			return
		}
		detail(w, 502, err)
		return
	}
	defer resp.Body.Close()
	// Kev returns a non-streaming JSON response. Buffer before committing the
	// status so a restart during body receipt still produces a clean 503.
	responseBody, err := io.ReadAll(resp.Body)
	if errors.Is(context.Cause(ctx), errRestart) {
		detail(w, 503, errRestart)
		return
	}
	if err != nil {
		detail(w, 502, err)
		return
	}
	for _, key := range []string{"Content-Type", "x-typesafe-request-id"} {
		if value := resp.Header.Get(key); value != "" {
			w.Header().Set(key, value)
		}
	}
	w.WriteHeader(resp.StatusCode)
	w.Write(responseBody)
}
