package main

import (
	"strings"
	"testing"

	"github.com/openziti/agora/internal/api"
)

func TestResolveServePlan(t *testing.T) {
	t.Parallel()

	proxy := func() *api.Tunnel {
		return &api.Tunnel{
			Name:          "pgw",
			Kind:          api.TunnelKindProxy,
			Mode:          api.TunnelModeHTTP,
			BackendTarget: api.NewOptString("http://127.0.0.1:8080"),
		}
	}

	t.Run("missing tunnel requires mode and backend", func(t *testing.T) {
		mode, backend, create, err := resolveServePlan(nil, "http", "http://127.0.0.1:8080")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !create || mode != "http" || backend != "http://127.0.0.1:8080" {
			t.Fatalf("unexpected plan: mode=%q backend=%q create=%v", mode, backend, create)
		}
	})

	t.Run("missing tunnel without mode errors", func(t *testing.T) {
		if _, _, _, err := resolveServePlan(nil, "", "http://127.0.0.1:8080"); err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("missing tunnel without backend errors", func(t *testing.T) {
		if _, _, _, err := resolveServePlan(nil, "http", ""); err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("existing proxy reused without flags", func(t *testing.T) {
		mode, backend, create, err := resolveServePlan(proxy(), "", "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if create {
			t.Fatal("expected create=false for existing tunnel")
		}
		if mode != "http" || backend != "http://127.0.0.1:8080" {
			t.Fatalf("unexpected plan: mode=%q backend=%q", mode, backend)
		}
	})

	t.Run("direct tunnel rejected", func(t *testing.T) {
		direct := &api.Tunnel{Name: "gw", Kind: api.TunnelKindDirect, Mode: api.TunnelModeHTTP}
		_, _, _, err := resolveServePlan(direct, "", "")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "direct") {
			t.Fatalf("unexpected error message: %q", err.Error())
		}
	})

	t.Run("mode mismatch errors", func(t *testing.T) {
		if _, _, _, err := resolveServePlan(proxy(), "tcp", ""); err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("backend mismatch errors", func(t *testing.T) {
		if _, _, _, err := resolveServePlan(proxy(), "", "http://other:9090"); err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("matching flags accepted", func(t *testing.T) {
		if _, _, _, err := resolveServePlan(proxy(), "http", "http://127.0.0.1:8080"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}
