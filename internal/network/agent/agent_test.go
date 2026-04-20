package agent

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/openziti/agora/environment"
	"github.com/openziti/agora/environment/env_core"
	networkpb "github.com/openziti/agora/internal/network/agent/pb"
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

	_, client, _, cleanup := startBufferedTestAgent(t)
	defer cleanup()

	if _, err := client.EnsureServe(context.Background(), &networkpb.EnsureServeRequest{
		TunnelId:      "tt_test00000001",
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
	if len(status.Status.Serves) != 1 || status.Status.Serves[0].State != networkpb.RuntimeState_RUNTIME_STATE_CONFIGURED {
		t.Fatalf("unexpected serve status: %#v", status.Status.Serves)
	}
	if len(status.Status.Connects) != 1 || status.Status.Connects[0].State != networkpb.RuntimeState_RUNTIME_STATE_CONFIGURED {
		t.Fatalf("unexpected connect status: %#v", status.Status.Connects)
	}

	loadedRoot := loadTestRoot(t)
	if loadedRoot.Network() == nil || len(loadedRoot.Network().Serves) != 1 || len(loadedRoot.Network().Connects) != 1 {
		t.Fatalf("unexpected persisted network state: %#v", loadedRoot.Network())
	}

	if _, err := client.RemoveServe(context.Background(), &networkpb.RemoveServeRequest{TunnelId: "tt_test00000001"}); err != nil {
		t.Fatalf("remove serve: %v", err)
	}
	if _, err := client.RemoveConnect(context.Background(), &networkpb.RemoveConnectRequest{
		TunnelId:      "tt_test00000001",
		ListenAddress: "127.0.0.1:8080",
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

	agent, err := New()
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

func TestPingAndStatusReturnErrNotRunningWhenSocketMissing(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "missing.sock")

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if _, err := Ping(ctx, socketPath); !errors.Is(err, ErrNotRunning) {
		t.Fatalf("expected ErrNotRunning from Ping, got %v", err)
	}
	if _, err := Status(ctx, socketPath); !errors.Is(err, ErrNotRunning) {
		t.Fatalf("expected ErrNotRunning from Status, got %v", err)
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

	_, client, _, cleanup := startBufferedTestAgentWithOptions(t, func(agent *Agent) {
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

	_, client, _, cleanup := startBufferedTestAgentWithOptions(t, func(agent *Agent) {
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

	_, client, _, cleanup := startBufferedTestAgentWithOptions(t, func(agent *Agent) {
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

	_, client, _, cleanup := startBufferedTestAgentWithOptions(t, func(agent *Agent) {
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

func startBufferedTestAgent(t *testing.T) (*Agent, networkpb.NetworkServiceClient, <-chan error, func()) {
	return startBufferedTestAgentWithOptions(t, nil)
}

func startBufferedTestAgentWithOptions(t *testing.T, configure func(*Agent)) (*Agent, networkpb.NetworkServiceClient, <-chan error, func()) {
	t.Helper()

	agent, err := New()
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
