package winet

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestProperties_Resolve(t *testing.T) {
	props := Properties{
		"I18N_COMMON_AC_VOLTAGE":            "AC voltage",
		"I18N_COMMON_GROUP_BUNCH_TITLE_AND": "String {0}",
		"I18N_COMMON_TOTAL_ACTIVE_POWER":    "Total active power",
	}

	tests := []struct {
		key  string
		want string
	}{
		{"I18N_COMMON_AC_VOLTAGE", "AC voltage"},
		{"I18N_COMMON_GROUP_BUNCH_TITLE_AND", "String {0}"},
		{"I18N_NONEXISTENT_KEY", "I18N_NONEXISTENT_KEY"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			got := props.Resolve(tt.key)
			if got != tt.want {
				t.Errorf("Resolve(%q) = %q, want %q", tt.key, got, tt.want)
			}
		})
	}
}

func TestFetchProperties_PlainHTTP(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/i18n/en_US.properties" {
			w.WriteHeader(404)
			return
		}
		_, _ = w.Write([]byte("KEY1=Value1\nKEY2=Value2\n# comment\n\nKEY3=Value3\n"))
	}))
	defer server.Close()

	host := strings.TrimPrefix(server.URL, "http://")
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	props, ssl, err := FetchProperties(context.Background(), logger, host, "en_US", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ssl {
		t.Error("expected ssl=false")
	}
	if len(props) != 3 {
		t.Errorf("expected 3 properties, got %d", len(props))
	}
	if props["KEY1"] != "Value1" {
		t.Errorf("expected Value1, got %s", props["KEY1"])
	}
}

func TestFetchProperties_ForceSSL(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("KEY=SSLValue\n"))
	}))
	defer server.Close()

	host := strings.TrimPrefix(server.URL, "https://")
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	props, ssl, err := FetchProperties(context.Background(), logger, host, "en_US", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ssl {
		t.Error("expected ssl=true")
	}
	if props["KEY"] != "SSLValue" {
		t.Errorf("expected SSLValue, got %s", props["KEY"])
	}
}

func TestFetchProperties_FallbackToSSL(t *testing.T) {
	// TLS server won't respond to plain HTTP — triggers fallback.
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("KEY=FallbackValue\n"))
	}))
	defer server.Close()

	host := strings.TrimPrefix(server.URL, "https://")
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	props, ssl, err := FetchProperties(context.Background(), logger, host, "en_US", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ssl {
		t.Error("expected ssl=true after fallback")
	}
	if props["KEY"] != "FallbackValue" {
		t.Errorf("expected FallbackValue, got %s", props["KEY"])
	}
}

func TestFetchProperties_BothFail(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	_, _, err := FetchProperties(context.Background(), logger, "localhost:1", "en_US", false)
	if err == nil {
		t.Error("expected error when both HTTP and HTTPS fail")
	}
}

func TestFetchProperties_ForceSSLFail(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	_, _, err := FetchProperties(context.Background(), logger, "localhost:1", "en_US", true)
	if err == nil {
		t.Error("expected error when forced SSL fails")
	}
}

func TestFetchProperties_BadStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(500)
	}))
	defer server.Close()

	host := strings.TrimPrefix(server.URL, "http://")
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	_, _, err := FetchProperties(context.Background(), logger, host, "en_US", false)
	if err == nil {
		t.Error("expected error for 500 status")
	}
}

func TestFetchProperties_EmptyFile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(""))
	}))
	defer server.Close()

	host := strings.TrimPrefix(server.URL, "http://")
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	props, _, err := FetchProperties(context.Background(), logger, host, "en_US", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(props) != 0 {
		t.Errorf("expected 0 properties, got %d", len(props))
	}
}

func TestFetchProperties_MalformedLines(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("GOOD=value\nno_equals_sign\n=no_key\nALSO_GOOD=val2\n"))
	}))
	defer server.Close()

	host := strings.TrimPrefix(server.URL, "http://")
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	props, _, err := FetchProperties(context.Background(), logger, host, "en_US", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(props) != 3 {
		t.Errorf("expected 3 properties, got %d: %v", len(props), props)
	}
}
