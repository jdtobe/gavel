// Package console serves the embedded browser console and validates Kev calls.
package console

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/jdtobe/gavel/internal/status"
	"github.com/jdtobe/gavel/web"
)

const MaxBodyBytes = 10 << 20

type Server struct {
	provider status.Provider
	upstream func() string
	client   *http.Client
	mu       sync.Mutex
	inflight int
	changed  chan struct{}
}

func New(provider status.Provider, upstream func() string, client *http.Client) *Server {
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Minute}
	}
	return &Server{provider: provider, upstream: upstream, client: client, changed: make(chan struct{})}
}
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(web.FS))))
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		data, _ := web.FS.ReadFile("index.html")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(data)
	})
	mux.HandleFunc("GET /api/status", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, s.provider.Status()) })
	mux.HandleFunc("GET /api/models", s.models)
	mux.HandleFunc("POST /api/systemone", s.systemone)
	return mux
}
func (s *Server) Start(ctx context.Context) (string, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	server := &http.Server{Handler: s.Handler(), ReadHeaderTimeout: 5 * time.Second}
	go server.Serve(l)
	go func() {
		<-ctx.Done()
		closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server.Shutdown(closeCtx)
		server.Close()
	}()
	return "http://" + l.Addr().String(), nil
}
func (s *Server) WaitIdle(ctx context.Context) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		s.mu.Lock()
		n, changed := s.inflight, s.changed
		s.mu.Unlock()
		if n == 0 {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-changed:
		}
	}
}
func (s *Server) begin() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.provider.Status().Phase != status.PhaseReady {
		return false
	}
	s.inflight++
	return true
}
func (s *Server) end() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.inflight--
	close(s.changed)
	s.changed = make(chan struct{})
}
func writeJSON(w http.ResponseWriter, code int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(value)
}
func detail(w http.ResponseWriter, code int, err any) {
	writeJSON(w, code, map[string]string{"detail": fmt.Sprint(err)})
}
