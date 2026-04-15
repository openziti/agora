package env_v0

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openziti/agora/environment/env_core"
	"github.com/openziti/agora/internal/api"
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

func TestSetAndLoadNetworkState(t *testing.T) {
	rootPath := filepath.Join(t.TempDir(), ".agora")
	SetRootDirName(rootPath)

	root, err := Default()
	if err != nil {
		t.Fatalf("default root: %v", err)
	}
	if err := root.SetNetwork(&env_core.Network{
		Serves: []env_core.ManagedServe{{
			TunnelID:      "tt_test00000001",
			Name:          "gateway",
			Mode:          api.TunnelModeHTTP,
			BackendTarget: "https://backend.example",
			GrantEmails:   []string{"one@example.com", "two@example.com"},
		}},
		Connects: []env_core.ManagedConnect{{
			TunnelID:      "tt_test00000001",
			Name:          "gateway",
			ListenAddress: "127.0.0.1:8080",
		}},
	}); err != nil {
		t.Fatalf("set network: %v", err)
	}

	loaded, err := Load()
	if err != nil {
		t.Fatalf("load root: %v", err)
	}
	if loaded.Network() == nil {
		t.Fatal("expected loaded network")
	}
	if len(loaded.Network().Serves) != 1 || loaded.Network().Serves[0].Mode != api.TunnelModeHTTP {
		t.Fatalf("unexpected loaded serves: %#v", loaded.Network().Serves)
	}
	if len(loaded.Network().Connects) != 1 || loaded.Network().Connects[0].ListenAddress != "127.0.0.1:8080" {
		t.Fatalf("unexpected loaded connects: %#v", loaded.Network().Connects)
	}

	socketPath, err := loaded.NetworkSocketPath()
	if err != nil {
		t.Fatalf("network socket path: %v", err)
	}
	if socketPath != filepath.Join(rootPath, "network.sock") {
		t.Fatalf("unexpected socket path: %s", socketPath)
	}
}

func TestNetworkJSONUsesExpectedShape(t *testing.T) {
	rootPath := filepath.Join(t.TempDir(), ".agora")
	SetRootDirName(rootPath)

	root, err := Default()
	if err != nil {
		t.Fatalf("default root: %v", err)
	}
	if err := root.SetNetwork(&env_core.Network{
		Serves: []env_core.ManagedServe{{
			Name:          "gateway",
			Mode:          api.TunnelModeHTTP,
			BackendTarget: "https://backend.example",
		}},
	}); err != nil {
		t.Fatalf("set network: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(rootPath, "network.json"))
	if err != nil {
		t.Fatalf("read network file: %v", err)
	}
	content := string(data)

	if !strings.Contains(content, `"serves"`) {
		t.Fatalf("expected serves field in network.json: %s", content)
	}
	if strings.Contains(content, `"connects"`) {
		t.Fatalf("did not expect empty connects field in network.json: %s", content)
	}
	if strings.Contains(content, `"tunnel_id"`) {
		t.Fatalf("did not expect empty tunnel_id in network.json: %s", content)
	}
	if strings.Contains(content, `"grant_emails"`) {
		t.Fatalf("did not expect empty grant_emails in network.json: %s", content)
	}
}

func TestEnvRootJSONFilesUse0600(t *testing.T) {
	rootPath := filepath.Join(t.TempDir(), ".agora")
	SetRootDirName(rootPath)

	root, err := Default()
	if err != nil {
		t.Fatalf("default root: %v", err)
	}
	if err := root.SetConfig(&env_core.Config{APIEndpoint: "http://127.0.0.1:8080"}); err != nil {
		t.Fatalf("set config: %v", err)
	}
	if err := root.SetEnvironment(&env_core.Environment{
		EnvironmentID: "ev_test00000001",
		AccountToken:  "account-token",
		ZitiIdentity:  "ziti-identity",
		APIEndpoint:   "http://127.0.0.1:8080",
	}); err != nil {
		t.Fatalf("set environment: %v", err)
	}
	if err := root.SetNetwork(&env_core.Network{
		Serves: []env_core.ManagedServe{{
			Name:          "gateway",
			Mode:          api.TunnelModeHTTP,
			BackendTarget: "https://backend.example",
		}},
	}); err != nil {
		t.Fatalf("set network: %v", err)
	}

	for _, name := range []string{"metadata.json", "config.json", "environment.json", "network.json"} {
		info, err := os.Stat(filepath.Join(rootPath, name))
		if err != nil {
			t.Fatalf("stat %s: %v", name, err)
		}
		if got := info.Mode().Perm(); got != 0o600 {
			t.Fatalf("expected %s to use 0600, got %03o", name, got)
		}
	}
}
