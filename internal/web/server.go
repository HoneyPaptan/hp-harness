package web

import (
	"context"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	defaultHeartbeat = 15 * time.Second
	defaultQueue     = 1024
	shutdownWait     = 5 * time.Second
)

type Options struct {
	Backend   Backend
	Token     string
	Hosts     []string
	Queue     int
	Heartbeat time.Duration
}

type Server struct {
	mgr       *manager
	token     string
	hosts     map[string]struct{}
	static    map[string]staticFile
	heartbeat time.Duration
	closing   chan struct{}
	closeOnce sync.Once
}

func New(o Options) (*Server, error) {
	if o.Token == "" {
		return nil, errors.New("web: token required")
	}
	static, err := loadStatic()
	if err != nil {
		return nil, err
	}
	s := &Server{
		mgr: newManager(o.Backend, positive(o.Queue, defaultQueue)), token: o.Token,
		hosts: hostSet(o.Hosts), static: static, closing: make(chan struct{}),
		heartbeat: positive(o.Heartbeat, defaultHeartbeat),
	}
	return s, nil
}

func positive[T int | time.Duration](v, fallback T) T {
	if v > 0 {
		return v
	}
	return fallback
}

func hostSet(hosts []string) map[string]struct{} {
	out := make(map[string]struct{}, len(hosts))
	for _, h := range hosts {
		out[strings.ToLower(h)] = struct{}{}
	}
	return out
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/state", s.handleState)
	mux.HandleFunc("GET /api/runs", s.handleRuns)
	mux.HandleFunc("POST /api/runs", s.handleStart)
	mux.HandleFunc("POST /api/runs/{id}/messages", s.handleMessage)
	mux.HandleFunc("POST /api/runs/{id}/stop", s.handleStop)
	mux.HandleFunc("POST /api/runs/{id}/model", s.handleModel)
	mux.HandleFunc("GET /api/runs/{id}/events", s.handleEvents)
	mux.HandleFunc("GET /", s.handleStatic)
	return s.guard(mux)
}

func (s *Server) Close() {
	s.closeOnce.Do(func() {
		s.mgr.close()
		close(s.closing)
	})
}

func (s *Server) Serve(ctx context.Context, ln net.Listener) error {
	hs := &http.Server{
		Handler: s.Handler(), ReadHeaderTimeout: 10 * time.Second,
		ErrorLog: log.New(io.Discard, "", 0),
	}
	failed := make(chan error, 1)
	go func() { failed <- hs.Serve(ln) }()
	select {
	case err := <-failed:
		s.Close()
		return err
	case <-ctx.Done():
	}
	s.Close()
	stop, cancel := context.WithTimeout(context.Background(), shutdownWait)
	defer cancel()
	return hs.Shutdown(stop)
}
