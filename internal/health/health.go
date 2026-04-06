package health

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

// Checker is a named health check function.
type Checker interface {
	Healthy(ctx context.Context) error
}

// CheckerFunc adapts a function to the Checker interface.
type CheckerFunc func(ctx context.Context) error

func (f CheckerFunc) Healthy(ctx context.Context) error { return f(ctx) }

// Server provides HTTP health check endpoints.
type Server struct {
	logger   *slog.Logger
	port     int
	mu       sync.RWMutex
	checkers map[string]Checker
	server   *http.Server
}

// NewServer creates a health check HTTP server.
func NewServer(logger *slog.Logger, port int) *Server {
	return &Server{
		logger:   logger,
		port:     port,
		checkers: make(map[string]Checker),
	}
}

// Register adds a named health checker.
func (s *Server) Register(name string, checker Checker) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.checkers[name] = checker
}

type checkResult struct {
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

type healthResponse struct {
	Status string                 `json:"status"`
	Checks map[string]checkResult `json:"checks"`
}

const (
	statusOK    = "ok"
	statusError = "error"
)

// Start runs the health check HTTP server.
func (s *Server) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.handleHealth)
	mux.HandleFunc("/readyz", s.handleHealth)

	s.server = &http.Server{
		Addr:              fmt.Sprintf(":%d", s.port),
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	s.logger.Info("health server starting", "port", s.port)
	if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("health server: %w", err)
	}
	return nil
}

// Shutdown gracefully stops the health server.
func (s *Server) Shutdown(ctx context.Context) error {
	if s.server == nil {
		return nil
	}
	return s.server.Shutdown(ctx)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	s.mu.RLock()
	checkers := make(map[string]Checker, len(s.checkers))
	for k, v := range s.checkers {
		checkers[k] = v
	}
	s.mu.RUnlock()

	resp := healthResponse{
		Status: statusOK,
		Checks: make(map[string]checkResult),
	}

	for name, checker := range checkers {
		if err := checker.Healthy(ctx); err != nil {
			resp.Status = statusError
			resp.Checks[name] = checkResult{Status: statusError, Error: err.Error()}
		} else {
			resp.Checks[name] = checkResult{Status: statusOK}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if resp.Status != statusOK {
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	_ = json.NewEncoder(w).Encode(resp)
}
