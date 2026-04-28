// Package live implements thin HTTP adapters that translate the four
// public-API upstreams (Yahoo Finance, Open-Meteo, Wikipedia Pageviews,
// GDELT) into the per-capability response shapes defined in
// internal/payloads. Each adapter is intentionally minimal: a single
// HTTP GET with a short timeout, a forgiving JSON unmarshal, and a
// projection into the typed Go shape. Callers are expected to wrap
// these in a snapshot-fallback layer.
package live

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const userAgent = "agora-macro-pulse/1.0 (+https://github.com/openziti/agora; demo)"

// DefaultTimeout is the per-request timeout for live upstream calls.
// Sized for a single GET on a public endpoint, not bulk fetches.
const DefaultTimeout = 8 * time.Second

// httpClient returns an http.Client with sensible defaults for the
// public-API calls.
func httpClient() *http.Client {
	return &http.Client{Timeout: DefaultTimeout}
}

// getJSON performs a GET, decodes the body as JSON into out, and
// returns a wrapped error on non-2xx responses or transport errors.
func getJSON(ctx context.Context, url string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("build request %s: %w", url, err)
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")
	resp, err := httpClient().Do(req)
	if err != nil {
		return fmt.Errorf("get %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("get %s: status %d: %s", url, resp.StatusCode, string(body))
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode %s: %w", url, err)
	}
	return nil
}
