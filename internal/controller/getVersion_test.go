package controller

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/michaelquigley/push/build"
	"github.com/openziti/agora/internal/api"
)

// TestGetVersionEndpointUnauthenticated verifies that GET /v1/version is reachable through
// the full controller middleware + ogen security stack with no token and no cookie, and
// returns a populated version. The handler never touches the store, so this runs without a
// database. The tests in this file mutate the process-global push build vars, so they are
// intentionally not parallel.
func TestGetVersionEndpointUnauthenticated(t *testing.T) {
	svc := &Service{}
	apiHandler, err := NewHandler(svc)
	if err != nil {
		t.Fatalf("new handler: %v", err)
	}
	ts := httptest.NewServer(authMiddlewareStack(svc, false, apiHandler))
	defer ts.Close()

	req, err := http.NewRequest(http.MethodGet, ts.URL+"/v1/version", nil)
	if err != nil {
		t.Fatalf("new version request: %v", err)
	}
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatalf("version request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read version response: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected unauthenticated version 200, got %d body=%q", resp.StatusCode, body)
	}

	var info api.VersionInfo
	if err := info.UnmarshalJSON(body); err != nil {
		t.Fatalf("decode version response: %v", err)
	}
	if info.Version == "" {
		t.Fatalf("expected non-empty version, got empty; body=%q", body)
	}
}

// TestGetVersionHandlerStampedFields verifies that stamped build vars surface in the
// VersionInfo and that unstamped optional fields are omitted.
func TestGetVersionHandlerStampedFields(t *testing.T) {
	prev := struct{ version, hash, date, builder, branch string }{
		build.Version, build.Hash, build.Date, build.Builder, build.Branch,
	}
	t.Cleanup(func() {
		build.Version, build.Hash, build.Date, build.Builder, build.Branch =
			prev.version, prev.hash, prev.date, prev.builder, prev.branch
	})

	build.Version = "v9.9.9"
	build.Hash = "deadbeef"
	build.Date = "2026-06-15T00:00:00Z"
	build.Builder = "goreleaser"
	build.Branch = "" // exercise the omit path for an unstamped optional field

	res, err := (&Service{}).GetVersion(context.Background())
	if err != nil {
		t.Fatalf("get version: %v", err)
	}
	info, ok := res.(*api.VersionInfo)
	if !ok {
		t.Fatalf("unexpected response type: %T", res)
	}

	if info.Version != "v9.9.9 [deadbeef]" {
		t.Fatalf("unexpected version %q", info.Version)
	}
	if v, ok := info.Hash.Get(); !ok || v != "deadbeef" {
		t.Fatalf("unexpected hash %q (set=%v)", v, ok)
	}
	if v, ok := info.Date.Get(); !ok || v != "2026-06-15T00:00:00Z" {
		t.Fatalf("unexpected date %q (set=%v)", v, ok)
	}
	if v, ok := info.Builder.Get(); !ok || v != "goreleaser" {
		t.Fatalf("unexpected builder %q (set=%v)", v, ok)
	}
	if _, ok := info.Branch.Get(); ok {
		t.Fatalf("expected branch to be unset")
	}
}
