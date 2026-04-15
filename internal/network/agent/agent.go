package agent

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/michaelquigley/df/dl"
	"github.com/openziti/agora/environment/env_core"
	networkpb "github.com/openziti/agora/internal/network/agent/pb"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var ErrAlreadyRunning = errors.New("agora network is already running")

type Agent struct {
	networkpb.UnimplementedNetworkServiceServer

	mu         sync.Mutex
	root       env_core.Root
	env        *env_core.Environment
	network    *env_core.Network
	socketPath string
	startedAt  time.Time
	pid        int

	grpcServer *grpc.Server
	shutdownCh chan struct{}
	stopOnce   sync.Once
}

func New() (*Agent, error) {
	root, env, networkState, err := loadRootState()
	if err != nil {
		return nil, err
	}
	socketPath, err := root.NetworkSocketPath()
	if err != nil {
		return nil, err
	}
	return &Agent{
		root:       root,
		env:        env,
		network:    networkState,
		socketPath: socketPath,
		startedAt:  time.Now().UTC(),
		pid:        os.Getpid(),
		shutdownCh: make(chan struct{}),
	}, nil
}

func (a *Agent) Run(ctx context.Context) error {
	if err := os.MkdirAll(filepath.Dir(a.socketPath), 0o700); err != nil {
		return fmt.Errorf("create network socket directory: %w", err)
	}

	if err := a.prepareSocket(ctx); err != nil {
		return err
	}

	listener, err := net.Listen("unix", a.socketPath)
	if err != nil {
		return fmt.Errorf("listen on network socket: %w", err)
	}
	defer func() {
		_ = listener.Close()
		_ = os.Remove(a.socketPath)
	}()
	if err := os.Chmod(a.socketPath, 0o600); err != nil {
		return fmt.Errorf("set network socket permissions: %w", err)
	}
	return a.runServer(ctx, listener, func() {
		_ = os.Remove(a.socketPath)
	})
}

func (a *Agent) runServer(ctx context.Context, listener net.Listener, cleanup func()) error {
	defer func() {
		_ = listener.Close()
		if cleanup != nil {
			cleanup()
		}
	}()

	server := grpc.NewServer()
	networkpb.RegisterNetworkServiceServer(server, a)
	a.grpcServer = server

	serveErrCh := make(chan error, 1)
	go func() {
		serveErrCh <- server.Serve(listener)
	}()

	dl.Infof("agora network started pid='%d' socket='%s'", a.pid, a.socketPath)
	defer dl.Infof("agora network stopped pid='%d' socket='%s'", a.pid, a.socketPath)

	select {
	case err := <-serveErrCh:
		if err != nil {
			return fmt.Errorf("serve network gRPC: %w", err)
		}
		return nil
	case <-ctx.Done():
		server.GracefulStop()
		<-serveErrCh
		return nil
	case <-a.shutdownCh:
		server.GracefulStop()
		<-serveErrCh
		return nil
	}
}

func (a *Agent) prepareSocket(ctx context.Context) error {
	pingCtx, cancel := context.WithTimeout(ctx, defaultDialTimeout)
	defer cancel()
	if _, err := Ping(pingCtx, a.socketPath); err == nil {
		return ErrAlreadyRunning
	}

	if socketExists(a.socketPath) {
		dl.Warnf("removing stale agora network socket '%s'", a.socketPath)
		if err := os.Remove(a.socketPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove stale network socket: %w", err)
		}
	}
	return nil
}

func (a *Agent) stop() {
	a.stopOnce.Do(func() {
		close(a.shutdownCh)
	})
}

func (a *Agent) snapshotStatusLocked() *networkpb.NetworkStatus {
	return networkStatusProto(a.pid, a.startedAt, a.socketPath, a.env, a.network)
}

func (a *Agent) Ping(context.Context, *networkpb.PingRequest) (*networkpb.PingResponse, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return &networkpb.PingResponse{
		Pid:       int32(a.pid),
		StartedAt: timestamppb.New(a.startedAt.UTC()),
	}, nil
}

func (a *Agent) Shutdown(context.Context, *networkpb.ShutdownRequest) (*networkpb.ShutdownResponse, error) {
	a.stop()
	return &networkpb.ShutdownResponse{}, nil
}

func (a *Agent) ReloadEnvironment(context.Context, *networkpb.ReloadEnvironmentRequest) (*networkpb.ReloadEnvironmentResponse, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	root, env, networkState, err := loadRootState()
	if err != nil {
		return nil, err
	}
	a.root = root
	a.env = env
	a.network = networkState
	return &networkpb.ReloadEnvironmentResponse{Status: a.snapshotStatusLocked()}, nil
}

func (a *Agent) GetNetworkStatus(_ context.Context, _ *networkpb.GetNetworkStatusRequest) (*networkpb.GetNetworkStatusResponse, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return &networkpb.GetNetworkStatusResponse{Status: a.snapshotStatusLocked()}, nil
}

func (a *Agent) EnsureServe(_ context.Context, req *networkpb.EnsureServeRequest) (*networkpb.EnsureServeResponse, error) {
	desired, err := validateDesiredServe(req)
	if err != nil {
		return nil, err
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	upsertServe(a.network, desired)
	if err := persistNetwork(a.root, a.network); err != nil {
		return nil, err
	}
	return &networkpb.EnsureServeResponse{Serve: serveStatusProto(desired)}, nil
}

func (a *Agent) RemoveServe(_ context.Context, req *networkpb.RemoveServeRequest) (*networkpb.RemoveServeResponse, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	removeServe(a.network, req.TunnelId, req.Name)
	if err := persistNetwork(a.root, a.network); err != nil {
		return nil, err
	}
	return &networkpb.RemoveServeResponse{}, nil
}

func (a *Agent) ListServes(_ context.Context, _ *networkpb.ListServesRequest) (*networkpb.ListServesResponse, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	resp := &networkpb.ListServesResponse{
		Serves: make([]*networkpb.ManagedServeStatus, 0, len(a.network.Serves)),
	}
	for _, serve := range a.network.Serves {
		resp.Serves = append(resp.Serves, serveStatusProto(serve))
	}
	return resp, nil
}

func (a *Agent) EnsureConnect(_ context.Context, req *networkpb.EnsureConnectRequest) (*networkpb.EnsureConnectResponse, error) {
	desired, err := validateDesiredConnect(req)
	if err != nil {
		return nil, err
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	upsertConnect(a.network, desired)
	if err := persistNetwork(a.root, a.network); err != nil {
		return nil, err
	}
	return &networkpb.EnsureConnectResponse{Connect: connectStatusProto(desired)}, nil
}

func (a *Agent) RemoveConnect(_ context.Context, req *networkpb.RemoveConnectRequest) (*networkpb.RemoveConnectResponse, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	removeConnect(a.network, req.TunnelId, req.Name, req.ListenAddress)
	if err := persistNetwork(a.root, a.network); err != nil {
		return nil, err
	}
	return &networkpb.RemoveConnectResponse{}, nil
}

func (a *Agent) ListConnects(_ context.Context, _ *networkpb.ListConnectsRequest) (*networkpb.ListConnectsResponse, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	resp := &networkpb.ListConnectsResponse{
		Connects: make([]*networkpb.ManagedConnectStatus, 0, len(a.network.Connects)),
	}
	for _, connect := range a.network.Connects {
		resp.Connects = append(resp.Connects, connectStatusProto(connect))
	}
	return resp, nil
}
