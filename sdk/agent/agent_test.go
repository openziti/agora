package agent

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/openziti/agora/environment"
	"github.com/openziti/agora/environment/env_core"
	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/sdk/agent/networkpb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

func TestAgentRunStatusAndShutdown(t *testing.T) {
	rootPath := filepath.Join(t.TempDir(), ".agora")
	environment.SetRootDirName(rootPath)

	root := loadTestRoot(t)
	if err := root.SetEnvironment(&env_core.Environment{EnvironmentID: "ev_test00000001"}); err != nil {
		t.Fatalf("set environment: %v", err)
	}

	agent, client, _, cleanup := startBufferedTestAgent(t)
	defer cleanup()

	ping, err := client.Ping(context.Background(), &networkpb.PingRequest{})
	if err != nil {
		t.Fatalf("ping: %v", err)
	}
	if ping.Pid != int32(agent.pid) {
		t.Fatalf("unexpected ping pid: %d", ping.Pid)
	}

	status, err := client.GetNetworkStatus(context.Background(), &networkpb.GetNetworkStatusRequest{})
	if err != nil {
		t.Fatalf("get network status: %v", err)
	}
	if !status.Status.EnvironmentEnabled || status.Status.EnvironmentId != "ev_test00000001" {
		t.Fatalf("unexpected environment status: %#v", status.Status)
	}

	if _, err := client.Shutdown(context.Background(), &networkpb.ShutdownRequest{}); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
}

func TestAgentEnsureServeAndConnectPersistDesiredState(t *testing.T) {
	rootPath := filepath.Join(t.TempDir(), ".agora")
	environment.SetRootDirName(rootPath)

	root := loadTestRoot(t)
	if err := root.SetEnvironment(&env_core.Environment{
		EnvironmentID: "ev_test00000002",
		AccountToken:  "account-token",
		APIEndpoint:   "http://controller.example",
	}); err != nil {
		t.Fatalf("set environment: %v", err)
	}
	if err := root.SaveZitiIdentityNamed(environmentIdentityName, `{"ztAPI":"test"}`); err != nil {
		t.Fatalf("save identity: %v", err)
	}

	controller := &fakeTunnelController{
		startServe: func(context.Context, *env_core.Environment, env_core.ManagedServe) (*api.Tunnel, *api.TunnelServe, error) {
			return &api.Tunnel{
				ID:            "tt_test00000001",
				EnvironmentId: "ev_test00000002",
				Name:          "gateway",
				Mode:          api.TunnelModeHTTP,
				BackendTarget: api.NewOptString("https://backend.example"),
			}, &api.TunnelServe{ID: "ts_test00000001"}, nil
		},
		startConnect: func(context.Context, *env_core.Environment, env_core.ManagedConnect) (*api.Tunnel, *api.TunnelAttachment, error) {
			return &api.Tunnel{
				ID:            "tt_test00000001",
				EnvironmentId: "ev_test00000002",
				Name:          "gateway",
				Mode:          api.TunnelModeHTTP,
				BackendTarget: api.NewOptString("https://backend.example"),
			}, &api.TunnelAttachment{ID: "ta_test00000001", ListenAddress: "127.0.0.1:8080"}, nil
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
		GrantEmails:   []string{"one@example.com", "two@example.com"},
	}); err != nil {
		t.Fatalf("ensure serve: %v", err)
	}
	if _, err := client.EnsureConnect(context.Background(), &networkpb.EnsureConnectRequest{
		TunnelId:      "tt_test00000001",
		Name:          "gateway",
		ListenAddress: "127.0.0.1:8080",
	}); err != nil {
		t.Fatalf("ensure connect: %v", err)
	}

	status, err := client.GetNetworkStatus(context.Background(), &networkpb.GetNetworkStatusRequest{})
	if err != nil {
		t.Fatalf("get network status: %v", err)
	}
	if len(status.Status.Serves) != 1 || status.Status.Serves[0].State != networkpb.RuntimeState_RUNTIME_STATE_RUNNING {
		t.Fatalf("unexpected serve status: %#v", status.Status.Serves)
	}
	if status.Status.Serves[0].ServeId != "ts_test00000001" {
		t.Fatalf("unexpected serve id: %#v", status.Status.Serves[0])
	}
	if len(status.Status.Connects) != 1 || status.Status.Connects[0].State != networkpb.RuntimeState_RUNTIME_STATE_RUNNING {
		t.Fatalf("unexpected connect status: %#v", status.Status.Connects)
	}
	if status.Status.Connects[0].AttachmentId != "ta_test00000001" {
		t.Fatalf("unexpected attachment id: %#v", status.Status.Connects[0])
	}

	loadedRoot := loadTestRoot(t)
	if loadedRoot.Network() == nil || len(loadedRoot.Network().Serves) != 1 || len(loadedRoot.Network().Connects) != 1 {
		t.Fatalf("unexpected persisted network state: %#v", loadedRoot.Network())
	}

	if _, err := client.RemoveServe(context.Background(), &networkpb.RemoveServeRequest{ServeId: "ts_test00000001"}); err != nil {
		t.Fatalf("remove serve: %v", err)
	}
	if _, err := client.RemoveConnect(context.Background(), &networkpb.RemoveConnectRequest{
		AttachmentId: "ta_test00000001",
	}); err != nil {
		t.Fatalf("remove connect: %v", err)
	}

	loadedRoot = loadTestRoot(t)
	if loadedRoot.Network() != nil {
		t.Fatalf("expected network state to be removed, got %#v", loadedRoot.Network())
	}
}

func TestAgentReloadEnvironment(t *testing.T) {
	rootPath := filepath.Join(t.TempDir(), ".agora")
	environment.SetRootDirName(rootPath)

	_, client, _, cleanup := startBufferedTestAgent(t)
	defer cleanup()

	root := loadTestRoot(t)
	if err := root.SetEnvironment(&env_core.Environment{EnvironmentID: "ev_test00000009"}); err != nil {
		t.Fatalf("set environment: %v", err)
	}

	resp, err := client.ReloadEnvironment(context.Background(), &networkpb.ReloadEnvironmentRequest{})
	if err != nil {
		t.Fatalf("reload environment: %v", err)
	}
	if !resp.Status.EnvironmentEnabled || resp.Status.EnvironmentId != "ev_test00000009" {
		t.Fatalf("unexpected reload status: %#v", resp.Status)
	}
}

func TestAgentPrepareSocketRemovesStaleSocket(t *testing.T) {
	rootPath := filepath.Join(t.TempDir(), ".agora")
	environment.SetRootDirName(rootPath)

	root := loadTestRoot(t)
	socketPath, err := root.NetworkSocketPath()
	if err != nil {
		t.Fatalf("network socket path: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(socketPath), 0o700); err != nil {
		t.Fatalf("mkdir socket dir: %v", err)
	}

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Skipf("unix sockets unavailable in this environment: %v", err)
	}
	_ = listener.Close()

	agent, err := NewDaemon()
	if err != nil {
		t.Fatalf("new agent: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := agent.prepareSocket(ctx); err != nil {
		t.Fatalf("prepare socket: %v", err)
	}
	if socketExists(socketPath) {
		t.Fatalf("expected stale socket to be removed: %s", socketPath)
	}
}

func TestAgentEnvironmentHeartbeatOnline(t *testing.T) {
	rootPath := filepath.Join(t.TempDir(), ".agora")
	environment.SetRootDirName(rootPath)

	const envID = "ev_test00000021"
	var requests atomic.Int32

	root := loadTestRoot(t)
	if err := root.SetEnvironment(&env_core.Environment{
		EnvironmentID: envID,
		AccountToken:  "account-token",
		APIEndpoint:   "http://controller.example",
	}); err != nil {
		t.Fatalf("set environment: %v", err)
	}

	_, client, _, cleanup := startBufferedTestAgentWithOptions(t, func(agent *Runtime) {
		agent.heartbeatInterval = 20 * time.Millisecond
		agent.heartbeatTTL = 60 * time.Millisecond
		agent.heartbeatRequestTimeout = 50 * time.Millisecond
		agent.heartbeatSender = func(context.Context, *env_core.Environment) (bool, error) {
			requests.Add(1)
			return false, nil
		}
	})
	defer cleanup()

	status := waitForHeartbeatState(t, client, networkpb.EnvironmentHeartbeatState_ENVIRONMENT_HEARTBEAT_STATE_ONLINE)
	if status.LastSuccessAt == nil {
		t.Fatalf("expected last success timestamp in %#v", status)
	}
	if requests.Load() == 0 {
		t.Fatal("expected at least one heartbeat request")
	}
}

func TestAgentEnvironmentHeartbeatStaleAfterTransientFailures(t *testing.T) {
	rootPath := filepath.Join(t.TempDir(), ".agora")
	environment.SetRootDirName(rootPath)

	const envID = "ev_test00000022"

	root := loadTestRoot(t)
	if err := root.SetEnvironment(&env_core.Environment{
		EnvironmentID: envID,
		AccountToken:  "account-token",
		APIEndpoint:   "http://controller.example",
	}); err != nil {
		t.Fatalf("set environment: %v", err)
	}

	_, client, _, cleanup := startBufferedTestAgentWithOptions(t, func(agent *Runtime) {
		agent.heartbeatInterval = 20 * time.Millisecond
		agent.heartbeatTTL = 60 * time.Millisecond
		agent.heartbeatRequestTimeout = 50 * time.Millisecond
		agent.heartbeatSender = func(context.Context, *env_core.Environment) (bool, error) {
			return false, errors.New("controller unavailable")
		}
	})
	defer cleanup()

	status := waitForHeartbeatState(t, client, networkpb.EnvironmentHeartbeatState_ENVIRONMENT_HEARTBEAT_STATE_STALE)
	if !strings.Contains(status.LastError, "controller unavailable") {
		t.Fatalf("expected transient heartbeat error, got %#v", status)
	}
}

func TestAgentEnvironmentHeartbeatPermanentError(t *testing.T) {
	rootPath := filepath.Join(t.TempDir(), ".agora")
	environment.SetRootDirName(rootPath)

	const envID = "ev_test00000023"
	var requests atomic.Int32

	root := loadTestRoot(t)
	if err := root.SetEnvironment(&env_core.Environment{
		EnvironmentID: envID,
		AccountToken:  "account-token",
		APIEndpoint:   "http://controller.example",
	}); err != nil {
		t.Fatalf("set environment: %v", err)
	}

	_, client, _, cleanup := startBufferedTestAgentWithOptions(t, func(agent *Runtime) {
		agent.heartbeatInterval = 20 * time.Millisecond
		agent.heartbeatTTL = 60 * time.Millisecond
		agent.heartbeatRequestTimeout = 50 * time.Millisecond
		agent.heartbeatSender = func(context.Context, *env_core.Environment) (bool, error) {
			requests.Add(1)
			return true, errors.New("environment not found")
		}
	})
	defer cleanup()

	status := waitForHeartbeatState(t, client, networkpb.EnvironmentHeartbeatState_ENVIRONMENT_HEARTBEAT_STATE_ERROR)
	if !strings.Contains(status.LastError, "not found") {
		t.Fatalf("expected permanent heartbeat error, got %#v", status)
	}
	if requests.Load() != 1 {
		t.Fatalf("expected one heartbeat request before permanent stop, got %d", requests.Load())
	}
}

func TestAgentReloadEnvironmentDisablesHeartbeat(t *testing.T) {
	rootPath := filepath.Join(t.TempDir(), ".agora")
	environment.SetRootDirName(rootPath)

	const envID = "ev_test00000024"
	var requests atomic.Int32

	root := loadTestRoot(t)
	if err := root.SetEnvironment(&env_core.Environment{
		EnvironmentID: envID,
		AccountToken:  "account-token",
		APIEndpoint:   "http://controller.example",
	}); err != nil {
		t.Fatalf("set environment: %v", err)
	}

	_, client, _, cleanup := startBufferedTestAgentWithOptions(t, func(agent *Runtime) {
		agent.heartbeatInterval = 20 * time.Millisecond
		agent.heartbeatTTL = 60 * time.Millisecond
		agent.heartbeatRequestTimeout = 50 * time.Millisecond
		agent.heartbeatSender = func(context.Context, *env_core.Environment) (bool, error) {
			requests.Add(1)
			return false, nil
		}
	})
	defer cleanup()

	waitForHeartbeatState(t, client, networkpb.EnvironmentHeartbeatState_ENVIRONMENT_HEARTBEAT_STATE_ONLINE)

	if err := root.DeleteEnvironment(); err != nil {
		t.Fatalf("delete environment: %v", err)
	}
	if _, err := client.ReloadEnvironment(context.Background(), &networkpb.ReloadEnvironmentRequest{}); err != nil {
		t.Fatalf("reload environment: %v", err)
	}

	waitForHeartbeatState(t, client, networkpb.EnvironmentHeartbeatState_ENVIRONMENT_HEARTBEAT_STATE_DISABLED)

	time.Sleep(50 * time.Millisecond)
	stable := requests.Load()
	time.Sleep(80 * time.Millisecond)
	if got := requests.Load(); got != stable {
		t.Fatalf("expected heartbeat loop to stop after disable, got %d then %d requests", stable, got)
	}
}

func TestAgentRestoresDesiredServeAndConnectOnStart(t *testing.T) {
	rootPath := filepath.Join(t.TempDir(), ".agora")
	environment.SetRootDirName(rootPath)

	root := loadTestRoot(t)
	if err := root.SetEnvironment(&env_core.Environment{
		EnvironmentID: "ev_test00000025",
		AccountToken:  "account-token",
		APIEndpoint:   "http://controller.example",
	}); err != nil {
		t.Fatalf("set environment: %v", err)
	}
	if err := root.SaveZitiIdentityNamed(environmentIdentityName, `{"ztAPI":"test"}`); err != nil {
		t.Fatalf("save identity: %v", err)
	}
	if err := root.SetNetwork(&env_core.Network{
		Serves: []env_core.ManagedServe{{
			Name:          "gateway",
			Mode:          api.TunnelModeHTTP,
			BackendTarget: "https://backend.example",
		}},
		Connects: []env_core.ManagedConnect{{
			Name:          "gateway",
			ListenAddress: "127.0.0.1:8080",
		}},
	}); err != nil {
		t.Fatalf("set network: %v", err)
	}

	var serveStarts atomic.Int32
	var connectStarts atomic.Int32
	controller := &fakeTunnelController{
		startServe: func(context.Context, *env_core.Environment, env_core.ManagedServe) (*api.Tunnel, *api.TunnelServe, error) {
			serveStarts.Add(1)
			return &api.Tunnel{
				ID:            "tt_test00000025",
				EnvironmentId: "ev_test00000025",
				Name:          "gateway",
				Mode:          api.TunnelModeHTTP,
				BackendTarget: api.NewOptString("https://backend.example"),
			}, &api.TunnelServe{ID: "ts_test00000025"}, nil
		},
		startConnect: func(context.Context, *env_core.Environment, env_core.ManagedConnect) (*api.Tunnel, *api.TunnelAttachment, error) {
			connectStarts.Add(1)
			return &api.Tunnel{
				ID:            "tt_test00000025",
				EnvironmentId: "ev_test00000025",
				Name:          "gateway",
				Mode:          api.TunnelModeHTTP,
				BackendTarget: api.NewOptString("https://backend.example"),
			}, &api.TunnelAttachment{ID: "ta_test00000025", ListenAddress: "127.0.0.1:8080"}, nil
		},
	}

	_, client, _, cleanup := startBufferedTestAgentWithOptions(t, func(agent *Runtime) {
		agent.controller = controller
		agent.runtimeHost = fakeRuntimeHost{}
	})
	defer cleanup()

	serve := waitForServeState(t, client, "gateway", networkpb.RuntimeState_RUNTIME_STATE_RUNNING)
	if serve.ServeId != "ts_test00000025" {
		t.Fatalf("unexpected serve restore status: %#v", serve)
	}
	connect := waitForConnectState(t, client, "gateway", "127.0.0.1:8080", networkpb.RuntimeState_RUNTIME_STATE_RUNNING)
	if connect.AttachmentId != "ta_test00000025" {
		t.Fatalf("unexpected connect restore status: %#v", connect)
	}
	if serveStarts.Load() != 1 {
		t.Fatalf("expected one serve restore attempt, got %d", serveStarts.Load())
	}
	if connectStarts.Load() != 1 {
		t.Fatalf("expected one connect restore attempt, got %d", connectStarts.Load())
	}
}

func TestAgentEnsureServeRetriesAfterInitialFailure(t *testing.T) {
	rootPath := filepath.Join(t.TempDir(), ".agora")
	environment.SetRootDirName(rootPath)

	root := loadTestRoot(t)
	if err := root.SetEnvironment(&env_core.Environment{
		EnvironmentID: "ev_test00000026",
		AccountToken:  "account-token",
		APIEndpoint:   "http://controller.example",
	}); err != nil {
		t.Fatalf("set environment: %v", err)
	}
	if err := root.SaveZitiIdentityNamed(environmentIdentityName, `{"ztAPI":"test"}`); err != nil {
		t.Fatalf("save identity: %v", err)
	}

	var attempts atomic.Int32
	controller := &fakeTunnelController{
		startServe: func(context.Context, *env_core.Environment, env_core.ManagedServe) (*api.Tunnel, *api.TunnelServe, error) {
			if attempts.Add(1) == 1 {
				return nil, nil, errors.New("controller unavailable")
			}
			return &api.Tunnel{
				ID:            "tt_test00000026",
				EnvironmentId: "ev_test00000026",
				Name:          "gateway",
				Mode:          api.TunnelModeHTTP,
				BackendTarget: api.NewOptString("https://backend.example"),
			}, &api.TunnelServe{ID: "ts_test00000026"}, nil
		},
		startConnect: func(context.Context, *env_core.Environment, env_core.ManagedConnect) (*api.Tunnel, *api.TunnelAttachment, error) {
			return nil, nil, errors.New("unexpected connect start")
		},
	}

	_, client, _, cleanup := startBufferedTestAgentWithOptions(t, func(agent *Runtime) {
		agent.controller = controller
		agent.runtimeHost = fakeRuntimeHost{}
		agent.retryInitialDelay = 40 * time.Millisecond
		agent.retryMaxDelay = 40 * time.Millisecond
		agent.retryJitter = 0
	})
	defer cleanup()

	if _, err := client.EnsureServe(context.Background(), &networkpb.EnsureServeRequest{
		Name:          "gateway",
		Mode:          "http",
		BackendTarget: "https://backend.example",
	}); err == nil {
		t.Fatal("expected initial ensure serve failure")
	}

	status := waitForServeRetry(t, client, "gateway", 1)
	if status.NextRetryAt == nil {
		t.Fatalf("expected scheduled retry, got %#v", status)
	}
	if attempts.Load() != 1 {
		t.Fatalf("expected one immediate start attempt, got %d", attempts.Load())
	}

	serve := waitForServeState(t, client, "gateway", networkpb.RuntimeState_RUNTIME_STATE_RUNNING)
	if serve.ServeId != "ts_test00000026" {
		t.Fatalf("unexpected recovered serve status: %#v", serve)
	}
	if attempts.Load() < 2 {
		t.Fatalf("expected background retry attempt, got %d", attempts.Load())
	}
}

func TestAgentRetriesAfterUnexpectedRuntimeExit(t *testing.T) {
	rootPath := filepath.Join(t.TempDir(), ".agora")
	environment.SetRootDirName(rootPath)

	root := loadTestRoot(t)
	if err := root.SetEnvironment(&env_core.Environment{
		EnvironmentID: "ev_test00000027",
		AccountToken:  "account-token",
		APIEndpoint:   "http://controller.example",
	}); err != nil {
		t.Fatalf("set environment: %v", err)
	}
	if err := root.SaveZitiIdentityNamed(environmentIdentityName, `{"ztAPI":"test"}`); err != nil {
		t.Fatalf("save identity: %v", err)
	}

	var starts atomic.Int32
	controller := &fakeTunnelController{
		startServe: func(context.Context, *env_core.Environment, env_core.ManagedServe) (*api.Tunnel, *api.TunnelServe, error) {
			start := starts.Add(1)
			return &api.Tunnel{
				ID:            "tt_test00000027",
				EnvironmentId: "ev_test00000027",
				Name:          "gateway",
				Mode:          api.TunnelModeHTTP,
				BackendTarget: api.NewOptString("https://backend.example"),
			}, &api.TunnelServe{ID: fmt.Sprintf("ts_test0000002%d", start)}, nil
		},
		startConnect: func(context.Context, *env_core.Environment, env_core.ManagedConnect) (*api.Tunnel, *api.TunnelAttachment, error) {
			return nil, nil, errors.New("unexpected connect start")
		},
	}

	var runtimeStarts atomic.Int32
	runtimeHost := fakeRuntimeHost{
		startServeFunc: func(ctx context.Context, _ string, _ *api.Tunnel) (runtimeHandle, error) {
			if runtimeStarts.Add(1) == 1 {
				return newTimedRuntimeHandle(ctx, 15*time.Millisecond, errors.New("backend crashed")), nil
			}
			return newFakeRuntimeHandle(ctx), nil
		},
	}

	_, client, _, cleanup := startBufferedTestAgentWithOptions(t, func(agent *Runtime) {
		agent.controller = controller
		agent.runtimeHost = runtimeHost
		agent.retryInitialDelay = 20 * time.Millisecond
		agent.retryMaxDelay = 20 * time.Millisecond
		agent.retryJitter = 0
	})
	defer cleanup()

	if _, err := client.EnsureServe(context.Background(), &networkpb.EnsureServeRequest{
		Name:          "gateway",
		Mode:          "http",
		BackendTarget: "https://backend.example",
	}); err != nil {
		t.Fatalf("ensure serve: %v", err)
	}

	serve := waitForServeID(t, client, "gateway", "ts_test00000022")
	if serve.ServeId != "ts_test00000022" {
		t.Fatalf("expected restarted serve id, got %#v", serve)
	}
	if starts.Load() < 2 {
		t.Fatalf("expected retry after runtime exit, got %d starts", starts.Load())
	}
}

func TestAgentServeHeartbeatCurrentResourceFailureReconcilesImmediately(t *testing.T) {
	rootPath := filepath.Join(t.TempDir(), ".agora")
	environment.SetRootDirName(rootPath)

	root := loadTestRoot(t)
	if err := root.SetEnvironment(&env_core.Environment{
		EnvironmentID: "ev_test00000028",
		AccountToken:  "account-token",
		APIEndpoint:   "http://controller.example",
	}); err != nil {
		t.Fatalf("set environment: %v", err)
	}
	if err := root.SaveZitiIdentityNamed(environmentIdentityName, `{"ztAPI":"test"}`); err != nil {
		t.Fatalf("save identity: %v", err)
	}

	var starts atomic.Int32
	controller := &fakeTunnelController{
		startServe: func(context.Context, *env_core.Environment, env_core.ManagedServe) (*api.Tunnel, *api.TunnelServe, error) {
			start := starts.Add(1)
			return &api.Tunnel{
				ID:            "tt_test00000028",
				EnvironmentId: "ev_test00000028",
				Name:          "gateway",
				Mode:          api.TunnelModeHTTP,
				BackendTarget: api.NewOptString("https://backend.example"),
			}, &api.TunnelServe{ID: fmt.Sprintf("ts_test0000003%d", start)}, nil
		},
		heartbeatServe: func(context.Context, *env_core.Environment, string) error {
			if starts.Load() == 1 {
				return newControllerError(controllerErrorKindCurrentRuntime, "serve not found")
			}
			return nil
		},
		startConnect: func(context.Context, *env_core.Environment, env_core.ManagedConnect) (*api.Tunnel, *api.TunnelAttachment, error) {
			return nil, nil, errors.New("unexpected connect start")
		},
	}

	_, client, _, cleanup := startBufferedTestAgentWithOptions(t, func(agent *Runtime) {
		agent.controller = controller
		agent.runtimeHost = fakeRuntimeHost{}
		agent.serveHeartbeatInterval = 15 * time.Millisecond
		agent.retryInitialDelay = 200 * time.Millisecond
		agent.retryMaxDelay = 200 * time.Millisecond
		agent.retryJitter = 0
	})
	defer cleanup()

	if _, err := client.EnsureServe(context.Background(), &networkpb.EnsureServeRequest{
		Name:          "gateway",
		Mode:          "http",
		BackendTarget: "https://backend.example",
	}); err != nil {
		t.Fatalf("ensure serve: %v", err)
	}

	serve := waitForServeID(t, client, "gateway", "ts_test00000032")
	if serve.ServeId != "ts_test00000032" {
		t.Fatalf("expected heartbeat-triggered restart, got %#v", serve)
	}
	if starts.Load() < 2 {
		t.Fatalf("expected immediate reconcile after heartbeat failure, got %d starts", starts.Load())
	}
}

func TestAgentReloadEnvironmentRecoversMissingPrerequisites(t *testing.T) {
	rootPath := filepath.Join(t.TempDir(), ".agora")
	environment.SetRootDirName(rootPath)

	root := loadTestRoot(t)
	if err := root.SetNetwork(&env_core.Network{
		Serves: []env_core.ManagedServe{{
			Name:          "gateway",
			Mode:          api.TunnelModeHTTP,
			BackendTarget: "https://backend.example",
		}},
	}); err != nil {
		t.Fatalf("set network: %v", err)
	}

	var starts atomic.Int32
	controller := &fakeTunnelController{
		startServe: func(context.Context, *env_core.Environment, env_core.ManagedServe) (*api.Tunnel, *api.TunnelServe, error) {
			starts.Add(1)
			return &api.Tunnel{
				ID:            "tt_test00000029",
				EnvironmentId: "ev_test00000029",
				Name:          "gateway",
				Mode:          api.TunnelModeHTTP,
				BackendTarget: api.NewOptString("https://backend.example"),
			}, &api.TunnelServe{ID: "ts_test00000029"}, nil
		},
		startConnect: func(context.Context, *env_core.Environment, env_core.ManagedConnect) (*api.Tunnel, *api.TunnelAttachment, error) {
			return nil, nil, errors.New("unexpected connect start")
		},
	}

	_, client, _, cleanup := startBufferedTestAgentWithOptions(t, func(agent *Runtime) {
		agent.controller = controller
		agent.runtimeHost = fakeRuntimeHost{}
		agent.retryInitialDelay = 20 * time.Millisecond
		agent.retryMaxDelay = 20 * time.Millisecond
		agent.retryJitter = 0
	})
	defer cleanup()

	serve := waitForServeState(t, client, "gateway", networkpb.RuntimeState_RUNTIME_STATE_ERROR)
	if serve.NextRetryAt != nil {
		t.Fatalf("expected no retry while prerequisites are missing, got %#v", serve)
	}
	if starts.Load() != 0 {
		t.Fatalf("expected no controller starts without prerequisites, got %d", starts.Load())
	}

	if err := root.SetEnvironment(&env_core.Environment{
		EnvironmentID: "ev_test00000029",
		AccountToken:  "account-token",
		APIEndpoint:   "http://controller.example",
	}); err != nil {
		t.Fatalf("set environment: %v", err)
	}
	if err := root.SaveZitiIdentityNamed(environmentIdentityName, `{"ztAPI":"test"}`); err != nil {
		t.Fatalf("save identity: %v", err)
	}

	if _, err := client.ReloadEnvironment(context.Background(), &networkpb.ReloadEnvironmentRequest{}); err != nil {
		t.Fatalf("reload environment: %v", err)
	}

	serve = waitForServeState(t, client, "gateway", networkpb.RuntimeState_RUNTIME_STATE_RUNNING)
	if serve.ServeId != "ts_test00000029" {
		t.Fatalf("unexpected recovered serve status: %#v", serve)
	}
	if starts.Load() != 1 {
		t.Fatalf("expected one controller start after reload, got %d", starts.Load())
	}
}

func TestAgentRemoveServeCancelsPendingRetry(t *testing.T) {
	rootPath := filepath.Join(t.TempDir(), ".agora")
	environment.SetRootDirName(rootPath)

	root := loadTestRoot(t)
	if err := root.SetEnvironment(&env_core.Environment{
		EnvironmentID: "ev_test00000030",
		AccountToken:  "account-token",
		APIEndpoint:   "http://controller.example",
	}); err != nil {
		t.Fatalf("set environment: %v", err)
	}
	if err := root.SaveZitiIdentityNamed(environmentIdentityName, `{"ztAPI":"test"}`); err != nil {
		t.Fatalf("save identity: %v", err)
	}

	var attempts atomic.Int32
	controller := &fakeTunnelController{
		startServe: func(context.Context, *env_core.Environment, env_core.ManagedServe) (*api.Tunnel, *api.TunnelServe, error) {
			attempts.Add(1)
			return nil, nil, errors.New("controller unavailable")
		},
		startConnect: func(context.Context, *env_core.Environment, env_core.ManagedConnect) (*api.Tunnel, *api.TunnelAttachment, error) {
			return nil, nil, errors.New("unexpected connect start")
		},
	}

	_, client, _, cleanup := startBufferedTestAgentWithOptions(t, func(agent *Runtime) {
		agent.controller = controller
		agent.runtimeHost = fakeRuntimeHost{}
		agent.retryInitialDelay = 40 * time.Millisecond
		agent.retryMaxDelay = 40 * time.Millisecond
		agent.retryJitter = 0
	})
	defer cleanup()

	if _, err := client.EnsureServe(context.Background(), &networkpb.EnsureServeRequest{
		Name:          "gateway",
		Mode:          "http",
		BackendTarget: "https://backend.example",
	}); err == nil {
		t.Fatal("expected initial ensure serve failure")
	}

	waitForServeRetry(t, client, "gateway", 1)

	if _, err := client.RemoveServe(context.Background(), &networkpb.RemoveServeRequest{Name: "gateway"}); err != nil {
		t.Fatalf("remove serve: %v", err)
	}

	time.Sleep(120 * time.Millisecond)

	status, err := client.GetNetworkStatus(context.Background(), &networkpb.GetNetworkStatusRequest{})
	if err != nil {
		t.Fatalf("get network status: %v", err)
	}
	if len(status.Status.Serves) != 0 {
		t.Fatalf("expected removed serve to stay removed, got %#v", status.Status.Serves)
	}
	if attempts.Load() != 1 {
		t.Fatalf("expected no additional retry after remove, got %d attempts", attempts.Load())
	}
}

func startBufferedTestAgent(t *testing.T) (*Runtime, networkpb.NetworkServiceClient, <-chan error, func()) {
	return startBufferedTestAgentWithOptions(t, nil)
}

func startBufferedTestAgentWithOptions(t *testing.T, configure func(*Runtime)) (*Runtime, networkpb.NetworkServiceClient, <-chan error, func()) {
	t.Helper()

	agent, err := NewDaemon()
	if err != nil {
		t.Fatalf("new agent: %v", err)
	}
	if configure != nil {
		configure(agent)
	}

	listener := bufconn.Listen(1024 * 1024)
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- agent.runServer(ctx, listener, nil)
	}()

	conn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return listener.Dial()
		}),
	)
	if err != nil {
		cancel()
		t.Fatalf("dial buffered agent: %v", err)
	}

	cleanup := func() {
		_, _ = networkpb.NewNetworkServiceClient(conn).Shutdown(context.Background(), &networkpb.ShutdownRequest{})
		cancel()
		_ = conn.Close()
		select {
		case err := <-errCh:
			if err != nil && !errors.Is(err, context.Canceled) {
				t.Fatalf("agent run: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for agent shutdown")
		}
	}

	return agent, networkpb.NewNetworkServiceClient(conn), errCh, cleanup
}

func loadTestRoot(t *testing.T) env_core.Root {
	t.Helper()
	root, err := environment.LoadRoot()
	if err != nil {
		t.Fatalf("load root: %v", err)
	}
	return root
}

func waitForHeartbeatState(t *testing.T, client networkpb.NetworkServiceClient, expected networkpb.EnvironmentHeartbeatState) *networkpb.EnvironmentHeartbeatStatus {
	t.Helper()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := client.GetNetworkStatus(context.Background(), &networkpb.GetNetworkStatusRequest{})
		if err != nil {
			t.Fatalf("get network status: %v", err)
		}
		if resp.Status.EnvironmentHeartbeat != nil && resp.Status.EnvironmentHeartbeat.State == expected {
			return resp.Status.EnvironmentHeartbeat
		}
		time.Sleep(10 * time.Millisecond)
	}

	resp, err := client.GetNetworkStatus(context.Background(), &networkpb.GetNetworkStatusRequest{})
	if err != nil {
		t.Fatalf("get final network status: %v", err)
	}
	t.Fatalf("timed out waiting for heartbeat state %s, got %#v", expected.String(), resp.Status.EnvironmentHeartbeat)
	return nil
}

func waitForServeState(t *testing.T, client networkpb.NetworkServiceClient, name string, expected networkpb.RuntimeState) *networkpb.ManagedServeStatus {
	t.Helper()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := client.GetNetworkStatus(context.Background(), &networkpb.GetNetworkStatusRequest{})
		if err != nil {
			t.Fatalf("get network status: %v", err)
		}
		for _, serve := range resp.Status.Serves {
			if strings.EqualFold(serve.Desired.Name, name) && serve.State == expected {
				return serve
			}
		}
		time.Sleep(10 * time.Millisecond)
	}

	resp, err := client.GetNetworkStatus(context.Background(), &networkpb.GetNetworkStatusRequest{})
	if err != nil {
		t.Fatalf("get final network status: %v", err)
	}
	t.Fatalf("timed out waiting for serve '%s' state %s, got %#v", name, expected.String(), resp.Status.Serves)
	return nil
}

func waitForConnectState(t *testing.T, client networkpb.NetworkServiceClient, name, listenAddress string, expected networkpb.RuntimeState) *networkpb.ManagedConnectStatus {
	t.Helper()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := client.GetNetworkStatus(context.Background(), &networkpb.GetNetworkStatusRequest{})
		if err != nil {
			t.Fatalf("get network status: %v", err)
		}
		for _, connect := range resp.Status.Connects {
			if strings.EqualFold(connect.Desired.Name, name) && connect.Desired.ListenAddress == listenAddress && connect.State == expected {
				return connect
			}
		}
		time.Sleep(10 * time.Millisecond)
	}

	resp, err := client.GetNetworkStatus(context.Background(), &networkpb.GetNetworkStatusRequest{})
	if err != nil {
		t.Fatalf("get final network status: %v", err)
	}
	t.Fatalf("timed out waiting for connect '%s' listen='%s' state %s, got %#v", name, listenAddress, expected.String(), resp.Status.Connects)
	return nil
}

func waitForServeID(t *testing.T, client networkpb.NetworkServiceClient, name, serveID string) *networkpb.ManagedServeStatus {
	t.Helper()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := client.GetNetworkStatus(context.Background(), &networkpb.GetNetworkStatusRequest{})
		if err != nil {
			t.Fatalf("get network status: %v", err)
		}
		for _, serve := range resp.Status.Serves {
			if strings.EqualFold(serve.Desired.Name, name) && serve.ServeId == serveID {
				return serve
			}
		}
		time.Sleep(10 * time.Millisecond)
	}

	resp, err := client.GetNetworkStatus(context.Background(), &networkpb.GetNetworkStatusRequest{})
	if err != nil {
		t.Fatalf("get final network status: %v", err)
	}
	t.Fatalf("timed out waiting for serve '%s' id '%s', got %#v", name, serveID, resp.Status.Serves)
	return nil
}

func waitForServeRetry(t *testing.T, client networkpb.NetworkServiceClient, name string, retryAttempt uint32) *networkpb.ManagedServeStatus {
	t.Helper()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := client.GetNetworkStatus(context.Background(), &networkpb.GetNetworkStatusRequest{})
		if err != nil {
			t.Fatalf("get network status: %v", err)
		}
		for _, serve := range resp.Status.Serves {
			if strings.EqualFold(serve.Desired.Name, name) && serve.RetryAttempt == retryAttempt {
				return serve
			}
		}
		time.Sleep(10 * time.Millisecond)
	}

	resp, err := client.GetNetworkStatus(context.Background(), &networkpb.GetNetworkStatusRequest{})
	if err != nil {
		t.Fatalf("get final network status: %v", err)
	}
	t.Fatalf("timed out waiting for serve '%s' retry attempt %d, got %#v", name, retryAttempt, resp.Status.Serves)
	return nil
}

type fakeTunnelController struct {
	startServe          func(context.Context, *env_core.Environment, env_core.ManagedServe) (*api.Tunnel, *api.TunnelServe, error)
	heartbeatServe      func(context.Context, *env_core.Environment, string) error
	stopServe           func(context.Context, *env_core.Environment, string) error
	startConnect        func(context.Context, *env_core.Environment, env_core.ManagedConnect) (*api.Tunnel, *api.TunnelAttachment, error)
	heartbeatAttachment func(context.Context, *env_core.Environment, string) error
	stopAttachment      func(context.Context, *env_core.Environment, string) error
}

func (f *fakeTunnelController) StartServe(ctx context.Context, env *env_core.Environment, desired env_core.ManagedServe) (*api.Tunnel, *api.TunnelServe, error) {
	return f.startServe(ctx, env, desired)
}

func (f *fakeTunnelController) HeartbeatServe(ctx context.Context, env *env_core.Environment, serveID string) error {
	if f.heartbeatServe == nil {
		return nil
	}
	return f.heartbeatServe(ctx, env, serveID)
}

func (f *fakeTunnelController) StopServe(ctx context.Context, env *env_core.Environment, serveID string) error {
	if f.stopServe == nil {
		return nil
	}
	return f.stopServe(ctx, env, serveID)
}

func (f *fakeTunnelController) StartConnect(ctx context.Context, env *env_core.Environment, desired env_core.ManagedConnect) (*api.Tunnel, *api.TunnelAttachment, error) {
	return f.startConnect(ctx, env, desired)
}

func (f *fakeTunnelController) HeartbeatAttachment(ctx context.Context, env *env_core.Environment, attachmentID string) error {
	if f.heartbeatAttachment == nil {
		return nil
	}
	return f.heartbeatAttachment(ctx, env, attachmentID)
}

func (f *fakeTunnelController) StopAttachment(ctx context.Context, env *env_core.Environment, attachmentID string) error {
	if f.stopAttachment == nil {
		return nil
	}
	return f.stopAttachment(ctx, env, attachmentID)
}

type fakeRuntimeHost struct {
	startServeFunc   func(context.Context, string, *api.Tunnel) (runtimeHandle, error)
	startConnectFunc func(context.Context, string, *api.Tunnel, string) (runtimeHandle, error)
}

func (f fakeRuntimeHost) StartServe(ctx context.Context, identityPath string, tunnel *api.Tunnel) (runtimeHandle, error) {
	if f.startServeFunc != nil {
		return f.startServeFunc(ctx, identityPath, tunnel)
	}
	return newFakeRuntimeHandle(ctx), nil
}

func (f fakeRuntimeHost) StartConnect(ctx context.Context, identityPath string, tunnel *api.Tunnel, listenAddress string) (runtimeHandle, error) {
	if f.startConnectFunc != nil {
		return f.startConnectFunc(ctx, identityPath, tunnel, listenAddress)
	}
	return newFakeRuntimeHandle(ctx), nil
}

type fakeRuntimeHandle struct {
	done chan error
}

func newFakeRuntimeHandle(ctx context.Context) runtimeHandle {
	done := make(chan error, 1)
	go func() {
		defer close(done)
		<-ctx.Done()
		done <- nil
	}()
	return &fakeRuntimeHandle{done: done}
}

func (h *fakeRuntimeHandle) Done() <-chan error {
	return h.done
}

func newTimedRuntimeHandle(ctx context.Context, delay time.Duration, result error) runtimeHandle {
	done := make(chan error, 1)
	go func() {
		defer close(done)
		select {
		case <-ctx.Done():
			done <- nil
		case <-time.After(delay):
			done <- result
		}
	}()
	return &fakeRuntimeHandle{done: done}
}
