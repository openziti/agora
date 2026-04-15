package networkagent

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/openziti/agora/environment"
	"github.com/openziti/agora/environment/env_core"
	networkpb "github.com/openziti/agora/internal/networkagent/pb"
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

func startBufferedTestAgent(t *testing.T) (*Agent, networkpb.NetworkServiceClient, <-chan error, func()) {
	t.Helper()

	agent, err := New()
	if err != nil {
		t.Fatalf("new agent: %v", err)
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
