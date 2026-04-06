package winet

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// Properties maps i18n keys to human-readable labels.
type Properties map[string]string

// FetchProperties retrieves the i18n properties file from the WiNet dongle.
// It auto-detects SSL: tries HTTP first, falls back to HTTPS.
// Returns the properties and whether SSL was used.
func FetchProperties(ctx context.Context, logger *slog.Logger, host, lang string, forceSSL bool) (Properties, bool, error) {
	if forceSSL {
		props, err := fetchPropertiesHTTP(ctx, host, lang, true)
		if err != nil {
			return nil, true, fmt.Errorf("fetch properties (ssl): %w", err)
		}
		return props, true, nil
	}

	// Try plain HTTP first.
	props, err := fetchPropertiesHTTP(ctx, host, lang, false)
	if err == nil {
		return props, false, nil
	}

	logger.Warn("plain HTTP failed, trying HTTPS", "error", err)

	// Fall back to HTTPS.
	props, err = fetchPropertiesHTTP(ctx, host, lang, true)
	if err != nil {
		return nil, false, fmt.Errorf("fetch properties (both HTTP and HTTPS failed): %w", err)
	}

	return props, true, nil
}

func fetchPropertiesHTTP(ctx context.Context, host, lang string, ssl bool) (Properties, error) {
	scheme := "http"
	if ssl {
		scheme = "https"
	}
	url := fmt.Sprintf("%s://%s/i18n/%s.properties", scheme, host, lang)

	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // self-signed certs on WiNet dongles
		},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP GET %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP GET %s: status %d", url, resp.StatusCode)
	}

	props := make(Properties)
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		props[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read properties: %w", err)
	}

	return props, nil
}

// Resolve looks up an i18n key in the properties map.
// Returns the translated name or the raw key if not found.
func (p Properties) Resolve(key string) string {
	if v, ok := p[key]; ok {
		return v
	}
	return key
}
