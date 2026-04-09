package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/michaelquigley/df/dd"
)

func TestLoadPreservesRootDefaultsWithoutOpenZiti(t *testing.T) {
	path := writeConfigFile(t, `
admin_tokens:
  - "admin-token"
store:
  dsn: "postgres://user:pass@localhost:5432/agora?sslmode=disable"
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.BindAddress != ":8080" {
		t.Fatalf("expected default bind address, got %q", cfg.BindAddress)
	}
	if cfg.OpenZiti != nil {
		t.Fatalf("expected open_ziti to remain nil")
	}
	if cfg.Store.MaxOpenConns != 4 {
		t.Fatalf("expected default max open conns, got %d", cfg.Store.MaxOpenConns)
	}
	if cfg.Store.MaxIdleConns != 4 {
		t.Fatalf("expected default max idle conns, got %d", cfg.Store.MaxIdleConns)
	}
	if cfg.Store.ConnMaxLifetime != time.Hour {
		t.Fatalf("expected default conn max lifetime, got %v", cfg.Store.ConnMaxLifetime)
	}
}

func TestLoadAppliesOpenZitiDefaultsOnFreshOptionalSubStruct(t *testing.T) {
	path := writeConfigFile(t, `
store:
  dsn: "postgres://user:pass@localhost:5432/agora?sslmode=disable"
open_ziti:
  api_endpoint: "https://controller.example"
  auth:
    updb:
      username: "admin"
      password: "secret"
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.OpenZiti == nil {
		t.Fatal("expected open_ziti to be allocated")
	}
	if cfg.OpenZiti.RequestTimeout != 30*time.Second {
		t.Fatalf("expected default request timeout, got %v", cfg.OpenZiti.RequestTimeout)
	}
	if cfg.OpenZiti.OperationTimeout != 30*time.Second {
		t.Fatalf("expected default operation timeout, got %v", cfg.OpenZiti.OperationTimeout)
	}
	if cfg.OpenZiti.Auth.Mode != "updb" {
		t.Fatalf("expected default auth mode, got %q", cfg.OpenZiti.Auth.Mode)
	}
}

func TestLoadPreservesExplicitOpenZitiOverrides(t *testing.T) {
	path := writeConfigFile(t, `
bind_address: ":9090"
store:
  dsn: "postgres://user:pass@localhost:5432/agora?sslmode=disable"
open_ziti:
  api_endpoint: "https://controller.example"
  request_timeout: "45s"
  operation_timeout: "1m30s"
  auth:
    mode: "updb"
    updb:
      username: "admin"
      password: "secret"
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.BindAddress != ":9090" {
		t.Fatalf("expected explicit bind address, got %q", cfg.BindAddress)
	}
	if cfg.OpenZiti == nil {
		t.Fatal("expected open_ziti to be allocated")
	}
	if cfg.OpenZiti.RequestTimeout != 45*time.Second {
		t.Fatalf("expected explicit request timeout, got %v", cfg.OpenZiti.RequestTimeout)
	}
	if cfg.OpenZiti.OperationTimeout != 90*time.Second {
		t.Fatalf("expected explicit operation timeout, got %v", cfg.OpenZiti.OperationTimeout)
	}
	if cfg.OpenZiti.Auth.Mode != "updb" {
		t.Fatalf("expected explicit auth mode, got %q", cfg.OpenZiti.Auth.Mode)
	}
}

func TestLoadRequiresStoreBlock(t *testing.T) {
	path := writeConfigFile(t, `
bind_address: ":8080"
`)

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected missing dsn error")
	}

	var reqErr *dd.RequiredFieldError
	if !errors.As(err, &reqErr) {
		t.Fatalf("expected required field error, got %T: %v", err, err)
	}
}

func TestLoadRequiresStoreDSNWhenStoreBlockPresent(t *testing.T) {
	path := writeConfigFile(t, `
store:
  max_open_conns: 8
`)

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected missing dsn error")
	}

	var reqErr *dd.RequiredFieldError
	if !errors.As(err, &reqErr) {
		t.Fatalf("expected required field error, got %T: %v", err, err)
	}
}

func TestLoadRequiresOpenZitiAPIEndpointWhenOpenZitiBlockPresent(t *testing.T) {
	path := writeConfigFile(t, `
store:
  dsn: "postgres://user:pass@localhost:5432/agora?sslmode=disable"
open_ziti:
  auth:
    updb:
      username: "admin"
      password: "secret"
`)

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected missing api_endpoint error")
	}

	var reqErr *dd.RequiredFieldError
	if !errors.As(err, &reqErr) {
		t.Fatalf("expected required field error, got %T: %v", err, err)
	}
}

func TestLoadAllowsOpenZitiBlockWithoutUPDBCredentials(t *testing.T) {
	path := writeConfigFile(t, `
store:
  dsn: "postgres://user:pass@localhost:5432/agora?sslmode=disable"
open_ziti:
  api_endpoint: "https://controller.example"
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("expected config load to succeed, got %v", err)
	}
	if cfg.OpenZiti == nil {
		t.Fatal("expected open_ziti to be allocated")
	}
}

func writeConfigFile(t *testing.T, contents string) string {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "agora.yml")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write config file: %v", err)
	}
	return path
}
