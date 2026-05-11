package tunnel_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/openziti/agora/environment"
	"github.com/openziti/agora/environment/env_core"
	"github.com/openziti/agora/sdk/agent"
	"github.com/openziti/agora/sdk/agent/tunnel"
)

func ExampleEnsureServed() {
	var a *agent.Agent
	_, _ = tunnel.EnsureServed(context.Background(), a, tunnel.ServeSpec{
		Name:          "llm-gateway",
		Mode:          tunnel.ModeHTTP,
		BackendTarget: "http://127.0.0.1:8080",
	})
}

func TestPublicSurfaceCompilesWithoutProtoTypes(t *testing.T) {
	var serve func(context.Context, *agent.Agent, tunnel.ServeSpec) (*tunnel.ServeStatus, error)
	var connect func(context.Context, *agent.Agent, tunnel.ConnectSpec) (*tunnel.ConnectStatus, error)
	serve = tunnel.EnsureServed
	connect = tunnel.EnsureConnected
	if serve == nil || connect == nil {
		t.Fatal("expected tunnel functions")
	}
}

func TestPublicFunctionsReturnRuntimeMissingWithoutStartedRuntime(t *testing.T) {
	rootPath := filepath.Join(t.TempDir(), ".agora")
	environment.SetRootDirName(rootPath)
	root, err := environment.LoadRoot()
	if err != nil {
		t.Fatalf("load root: %v", err)
	}
	if err := root.SetEnvironment(&env_core.Environment{
		EnvironmentID: "ev_tunneltest01",
		AccountToken:  "account-token",
		APIEndpoint:   "http://controller.example",
	}); err != nil {
		t.Fatalf("set environment: %v", err)
	}

	a, err := agent.NewStandalone(agent.StandaloneOptions{Name: "gateway", EnvRoot: rootPath})
	if err != nil {
		t.Fatalf("new standalone: %v", err)
	}

	_, err = tunnel.EnsureServed(context.Background(), a, tunnel.ServeSpec{
		Name:          "gateway",
		Mode:          tunnel.ModeTCP,
		BackendTarget: "127.0.0.1:8080",
	})
	if !errors.Is(err, tunnel.ErrRuntimeMissing) {
		t.Fatalf("expected ErrRuntimeMissing, got %v", err)
	}
	_, err = tunnel.EnsureConnected(context.Background(), a, tunnel.ConnectSpec{
		Name:          "gateway",
		ListenAddress: "127.0.0.1:9080",
	})
	if !errors.Is(err, tunnel.ErrRuntimeMissing) {
		t.Fatalf("expected ErrRuntimeMissing, got %v", err)
	}
}
