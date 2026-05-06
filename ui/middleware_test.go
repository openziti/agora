//go:build !no_agora_ui

package ui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMiddlewareForwardsAPIRequests(t *testing.T) {
	var gotPath string
	handler := Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte("api"))
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/foo", nil))

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusAccepted)
	}
	if gotPath != "/v1/foo" {
		t.Fatalf("api handler path = %q, want %q", gotPath, "/v1/foo")
	}
	if body := rec.Body.String(); body != "api" {
		t.Fatalf("body = %q, want api", body)
	}
}

func TestMiddlewareServesSPAFallback(t *testing.T) {
	handler := Middleware(http.NotFoundHandler())

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/some-spa-route", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Fatalf("content-type = %q, want text/html", ct)
	}
	if body := rec.Body.String(); !strings.Contains(body, `<div id="root"></div>`) {
		t.Fatalf("fallback body does not look like index.html: %q", body)
	}
}
