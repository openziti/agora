package env_v0

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/openziti/agora/environment/env_core"
)

func TestSetAndLoadConfig(t *testing.T) {
	rootPath := filepath.Join(t.TempDir(), ".agora")
	SetRootDirName(rootPath)

	root, err := Default()
	if err != nil {
		t.Fatalf("default root: %v", err)
	}
	if err := root.SetConfig(&env_core.Config{APIEndpoint: "http://127.0.0.1:8080"}); err != nil {
		t.Fatalf("set config: %v", err)
	}

	loaded, err := Load()
	if err != nil {
		t.Fatalf("load root: %v", err)
	}
	if loaded.Config() == nil || loaded.Config().APIEndpoint != "http://127.0.0.1:8080" {
		t.Fatalf("unexpected loaded config: %#v", loaded.Config())
	}
	if _, err := os.Stat(filepath.Join(rootPath, "metadata.json")); err != nil {
		t.Fatalf("expected metadata file: %v", err)
	}
}

func TestAPIEndpointPrecedence(t *testing.T) {
	rootPath := filepath.Join(t.TempDir(), ".agora")
	SetRootDirName(rootPath)
	t.Setenv("AGORA_API_ENDPOINT", "")

	root, err := Default()
	if err != nil {
		t.Fatalf("default root: %v", err)
	}
	if err := root.SetConfig(&env_core.Config{APIEndpoint: "http://config.example"}); err != nil {
		t.Fatalf("set config: %v", err)
	}
	if err := root.SetEnvironment(&env_core.Environment{EnvironmentID: "ev_test00000001", APIEndpoint: "http://environment.example"}); err != nil {
		t.Fatalf("set environment: %v", err)
	}

	if got, from := root.APIEndpoint(); got != "http://environment.example" || from != "env" {
		t.Fatalf("expected env endpoint, got %q from %q", got, from)
	}

	t.Setenv("AGORA_API_ENDPOINT", "http://envvar.example")
	if got, from := root.APIEndpoint(); got != "http://envvar.example" || from != "AGORA_API_ENDPOINT" {
		t.Fatalf("expected env var endpoint, got %q from %q", got, from)
	}
}

func TestSetAndLoadEnvironmentIncludesEnvironmentID(t *testing.T) {
	rootPath := filepath.Join(t.TempDir(), ".agora")
	SetRootDirName(rootPath)

	root, err := Default()
	if err != nil {
		t.Fatalf("default root: %v", err)
	}
	if err := root.SetEnvironment(&env_core.Environment{
		EnvironmentID: "ev_test00000001",
		AccountToken:  "account-token",
		ZitiIdentity:  "ziti-identity",
		APIEndpoint:   "http://127.0.0.1:8080",
	}); err != nil {
		t.Fatalf("set environment: %v", err)
	}

	loaded, err := Load()
	if err != nil {
		t.Fatalf("load root: %v", err)
	}
	if loaded.Environment() == nil || loaded.Environment().EnvironmentID != "ev_test00000001" {
		t.Fatalf("unexpected loaded environment: %#v", loaded.Environment())
	}
}
