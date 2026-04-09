package automation

import (
	"context"
	"strings"
	"testing"
)

func TestNewClientRequiresUPDBUsername(t *testing.T) {
	_, err := newClient(context.Background(), &Config{
		APIEndpoint: "https://controller.example",
		Auth: AuthConfig{
			Mode: "updb",
			UPDB: UPDBAuthConfig{
				Password: "secret",
			},
		},
	}, nil)
	if err == nil {
		t.Fatal("expected username validation error")
	}
	if !strings.Contains(err.Error(), "auth.updb.username is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNewClientRequiresUPDBPassword(t *testing.T) {
	_, err := newClient(context.Background(), &Config{
		APIEndpoint: "https://controller.example",
		Auth: AuthConfig{
			Mode: "updb",
			UPDB: UPDBAuthConfig{
				Username: "admin",
			},
		},
	}, nil)
	if err == nil {
		t.Fatal("expected password validation error")
	}
	if !strings.Contains(err.Error(), "auth.updb.password is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}
