package main

import (
	"strings"
	"testing"

	"github.com/openziti/agora/internal/api"
)

func TestTunnelFromCreateResult(t *testing.T) {
	t.Parallel()

	t.Run("success returns tunnel", func(t *testing.T) {
		want := &api.Tunnel{ID: "tt_abcdef012345", Name: "gw"}
		got, err := tunnelFromCreateResult(want)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != want {
			t.Fatalf("expected tunnel %v, got %v", want, got)
		}
	})

	t.Run("conflict surfaces server message", func(t *testing.T) {
		_, err := tunnelFromCreateResult(&api.CreateTunnelConflict{Code: "conflict", Message: "tunnel name already exists"})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "tunnel name already exists") {
			t.Fatalf("unexpected error message: %q", err.Error())
		}
	})

	t.Run("not found errors", func(t *testing.T) {
		_, err := tunnelFromCreateResult(&api.CreateTunnelNotFound{Code: "not_found", Message: "environment not found"})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("unauthorized errors", func(t *testing.T) {
		_, err := tunnelFromCreateResult(&api.CreateTunnelUnauthorized{Code: "unauthorized", Message: "unauthorized"})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("internal server error errors", func(t *testing.T) {
		_, err := tunnelFromCreateResult(&api.CreateTunnelInternalServerError{Code: "internal_error", Message: "boom"})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}
