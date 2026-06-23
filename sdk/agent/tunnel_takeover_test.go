package agent

import (
	"context"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/openziti/agora/environment"
	"github.com/openziti/agora/environment/env_core"
	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/sdk/agent/networkpb"
)

// EnsureServe with takeover set dispatches the controller's Takeover for the tunnel by name, before
// the serve attempt runs.
func TestEnsureServeTakeoverInvokesControllerBeforeServe(t *testing.T) {
	rootPath := filepath.Join(t.TempDir(), ".agora")
	environment.SetRootDirName(rootPath)

	root := loadTestRoot(t)
	if err := root.SetEnvironment(&env_core.Environment{
		EnvironmentID: "ev_test00000040",
		AccountToken:  "account-token",
		APIEndpoint:   "http://controller.example",
	}); err != nil {
		t.Fatalf("set environment: %v", err)
	}
	if err := root.SaveZitiIdentityNamed(environmentIdentityName, `{"ztAPI":"test"}`); err != nil {
		t.Fatalf("save identity: %v", err)
	}

	var takeoverName atomic.Value
	var serveCalled atomic.Bool
	controller := &fakeTunnelController{
		takeover: func(_ context.Context, _ *env_core.Environment, name string) error {
			if serveCalled.Load() {
				t.Errorf("takeover must run before serve")
			}
			takeoverName.Store(name)
			return nil
		},
		startServe: func(context.Context, *env_core.Environment, env_core.ManagedServe) (*api.Tunnel, *api.TunnelServe, error) {
			serveCalled.Store(true)
			return &api.Tunnel{
					ID:            "tt_test00000040",
					Name:          "gateway",
					Mode:          api.TunnelModeHTTP,
					BackendTarget: api.NewOptString("https://backend.example"),
				},
				&api.TunnelServe{ID: "ts_test00000040"}, nil
		},
	}

	_, client, _, cleanup := startBufferedTestAgentWithOptions(t, func(agent *Runtime) {
		agent.controller = controller
		agent.runtimeHost = fakeRuntimeHost{}
	})
	defer cleanup()

	if _, err := client.EnsureServe(context.Background(), &networkpb.EnsureServeRequest{
		Name:          "gateway",
		Mode:          "http",
		BackendTarget: "https://backend.example",
		Takeover:      true,
	}); err != nil {
		t.Fatalf("ensure serve with takeover: %v", err)
	}

	if got, _ := takeoverName.Load().(string); got != "gateway" {
		t.Fatalf("expected takeover for 'gateway', got %q", got)
	}
	if !serveCalled.Load() {
		t.Fatalf("expected serve to run after takeover")
	}
}

// EnsureServe without takeover does not dispatch Takeover.
func TestEnsureServeWithoutTakeoverSkipsController(t *testing.T) {
	rootPath := filepath.Join(t.TempDir(), ".agora")
	environment.SetRootDirName(rootPath)

	root := loadTestRoot(t)
	if err := root.SetEnvironment(&env_core.Environment{
		EnvironmentID: "ev_test00000041",
		AccountToken:  "account-token",
		APIEndpoint:   "http://controller.example",
	}); err != nil {
		t.Fatalf("set environment: %v", err)
	}
	if err := root.SaveZitiIdentityNamed(environmentIdentityName, `{"ztAPI":"test"}`); err != nil {
		t.Fatalf("save identity: %v", err)
	}

	var takeoverCalled atomic.Bool
	controller := &fakeTunnelController{
		takeover: func(_ context.Context, _ *env_core.Environment, _ string) error {
			takeoverCalled.Store(true)
			return nil
		},
		startServe: func(context.Context, *env_core.Environment, env_core.ManagedServe) (*api.Tunnel, *api.TunnelServe, error) {
			return &api.Tunnel{
					ID:            "tt_test00000041",
					Name:          "gateway",
					Mode:          api.TunnelModeHTTP,
					BackendTarget: api.NewOptString("https://backend.example"),
				},
				&api.TunnelServe{ID: "ts_test00000041"}, nil
		},
	}

	_, client, _, cleanup := startBufferedTestAgentWithOptions(t, func(agent *Runtime) {
		agent.controller = controller
		agent.runtimeHost = fakeRuntimeHost{}
	})
	defer cleanup()

	if _, err := client.EnsureServe(context.Background(), &networkpb.EnsureServeRequest{
		Name:          "gateway",
		Mode:          "http",
		BackendTarget: "https://backend.example",
	}); err != nil {
		t.Fatalf("ensure serve: %v", err)
	}

	if takeoverCalled.Load() {
		t.Fatalf("expected no takeover when the flag is not set")
	}
}
