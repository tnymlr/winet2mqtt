package health

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestHealthHandler_AllHealthy(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	s := NewServer(logger, 0)
	s.Register("mqtt", CheckerFunc(func(_ context.Context) error { return nil }))
	s.Register("winet", CheckerFunc(func(_ context.Context) error { return nil }))

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	s.handleHealth(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp healthResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Status != "ok" {
		t.Errorf("expected status ok, got %s", resp.Status)
	}
	if resp.Checks["mqtt"].Status != "ok" {
		t.Errorf("expected mqtt ok, got %s", resp.Checks["mqtt"].Status)
	}
	if resp.Checks["winet"].Status != "ok" {
		t.Errorf("expected winet ok, got %s", resp.Checks["winet"].Status)
	}
}

func TestHealthHandler_OneUnhealthy(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	s := NewServer(logger, 0)
	s.Register("mqtt", CheckerFunc(func(_ context.Context) error { return nil }))
	s.Register("winet", CheckerFunc(func(_ context.Context) error {
		return fmt.Errorf("not connected")
	}))

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	s.handleHealth(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", w.Code)
	}

	var resp healthResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Status != "error" {
		t.Errorf("expected status error, got %s", resp.Status)
	}
	if resp.Checks["winet"].Status != "error" {
		t.Errorf("expected winet error, got %s", resp.Checks["winet"].Status)
	}
	if resp.Checks["winet"].Error != "not connected" {
		t.Errorf("expected error message 'not connected', got %s", resp.Checks["winet"].Error)
	}
}

func TestHealthHandler_NoCheckers(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	s := NewServer(logger, 0)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	w := httptest.NewRecorder()
	s.handleHealth(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}
