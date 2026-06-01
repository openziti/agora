package main

import (
	"testing"

	"github.com/openziti/agora/internal/api"
)

func TestAccountTokenFromLoginResult(t *testing.T) {
	t.Parallel()

	t.Run("success returns token", func(t *testing.T) {
		token, err := accountTokenFromLoginResult(&api.AccountTokenResponse{AccountToken: "act_abc123"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if token != "act_abc123" {
			t.Fatalf("expected token act_abc123, got %q", token)
		}
	})

	t.Run("unauthorized maps to credentials error", func(t *testing.T) {
		_, err := accountTokenFromLoginResult(&api.LoginUnauthorized{Code: "unauthorized", Message: "invalid credentials"})
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		if got := err.Error(); got != "login failed: invalid credentials" {
			t.Fatalf("unexpected error message: %q", got)
		}
	})

	t.Run("unexpected variant errors", func(t *testing.T) {
		_, err := accountTokenFromLoginResult(&api.LoginInternalServerError{Code: "internal_error", Message: "boom"})
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
	})
}
