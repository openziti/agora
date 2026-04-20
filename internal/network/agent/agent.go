package agent

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
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
	serves     map[string]*managedServe
	connects   map[string]*managedConnect
	socketPath string
	startedAt  time.Time
	pid        int

	now                      func() time.Time
	heartbeatInterval        time.Duration
	heartbeatTTL             time.Duration
	heartbeatRequestTimeout  time.Duration
	heartbeatSender          func(context.Context, *env_core.Environment) (bool, error)
	heartbeat                environmentHeartbeat
	controller               tunnelController
	runtimeHost              tunnelRuntimeHost
	serveHeartbeatInterval   time.Duration
	connectHeartbeatInterval time.Duration
	retryInitialDelay        time.Duration
	retryMaxDelay            time.Duration
	retryMultiplier          float64
	retryJitter              float64
	retryRand                func() float64

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
	agent := &Agent{
		root:                     root,
		env:                      env,
		network:                  networkState,
		serves:                   configuredServes(networkState),
		connects:                 configuredConnects(networkState),
		socketPath:               socketPath,
		startedAt:                time.Now().UTC(),
		pid:                      os.Getpid(),
		now:                      time.Now,
		heartbeatInterval:        defaultEnvironmentHeartbeatInterval,
		heartbeatTTL:             defaultEnvironmentHeartbeatTTL,
		heartbeatRequestTimeout:  defaultEnvironmentHeartbeatRequestTimeout,
		shutdownCh:               make(chan struct{}),
		controller:               apiTunnelController{},
		runtimeHost:              defaultRuntimeHost{},
		serveHeartbeatInterval:   tunnelServeHeartbeatInterval,
		connectHeartbeatInterval: tunnelAttachmentHeartbeatInterval,
		retryInitialDelay:        defaultTunnelRetryInitialDelay,
		retryMaxDelay:            defaultTunnelRetryMaxDelay,
		retryMultiplier:          defaultTunnelRetryMultiplier,
		retryJitter:              defaultTunnelRetryJitter,
		retryRand:                rand.Float64,
	}
	agent.heartbeatSender = agent.sendEnvironmentHeartbeat
	return agent, nil
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
		a.shutdownManagedActors()
		a.stopEnvironmentHeartbeatLoop()
		_ = listener.Close()
		if cleanup != nil {
			cleanup()
		}
	}()

	server := grpc.NewServer()
	networkpb.RegisterNetworkServiceServer(server, a)
	a.grpcServer = server
	a.mu.Lock()
	a.replaceEnvironmentHeartbeatLoopLocked()
	a.startConfiguredActorsLocked()
	a.mu.Unlock()

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
	status := networkStatusProto(a.pid, a.startedAt, a.socketPath, a.env, a.network, a.environmentHeartbeatStatusLocked(a.now().UTC()))
	for _, desired := range a.network.Serves {
		if _, actor := a.findServeActorLocked(desired.TunnelID, desired.Name); actor != nil {
			status.Serves = append(status.Serves, serveStatusProto(actor))
			continue
		}
		status.Serves = append(status.Serves, serveStatusProto(&managedServe{
			desired:  desired,
			state:    networkpb.RuntimeState_RUNTIME_STATE_CONFIGURED,
			tunnelID: desired.TunnelID,
		}))
	}
	for _, desired := range a.network.Connects {
		if _, actor := a.findConnectActorLocked(desired.TunnelID, desired.Name, desired.ListenAddress); actor != nil {
			status.Connects = append(status.Connects, connectStatusProto(actor))
			continue
		}
		status.Connects = append(status.Connects, connectStatusProto(&managedConnect{
			desired:  desired,
			state:    networkpb.RuntimeState_RUNTIME_STATE_CONFIGURED,
			tunnelID: desired.TunnelID,
		}))
	}
	return status
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
	a.shutdownManagedActors()

	a.mu.Lock()
	defer a.mu.Unlock()

	root, env, networkState, err := loadRootState()
	if err != nil {
		return nil, err
	}
	a.root = root
	a.env = env
	a.network = networkState
	a.serves = configuredServes(networkState)
	a.connects = configuredConnects(networkState)
	a.replaceEnvironmentHeartbeatLoopLocked()
	a.startConfiguredActorsLocked()
	return &networkpb.ReloadEnvironmentResponse{Status: a.snapshotStatusLocked()}, nil
}

func (a *Agent) GetNetworkStatus(_ context.Context, _ *networkpb.GetNetworkStatusRequest) (*networkpb.GetNetworkStatusResponse, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return &networkpb.GetNetworkStatusResponse{Status: a.snapshotStatusLocked()}, nil
}

func (a *Agent) ListServes(_ context.Context, _ *networkpb.ListServesRequest) (*networkpb.ListServesResponse, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	resp := &networkpb.ListServesResponse{
		Serves: make([]*networkpb.ManagedServeStatus, 0, len(a.network.Serves)),
	}
	for _, serve := range a.network.Serves {
		if _, actor := a.findServeActorLocked(serve.TunnelID, serve.Name); actor != nil {
			resp.Serves = append(resp.Serves, serveStatusProto(actor))
			continue
		}
		resp.Serves = append(resp.Serves, serveStatusProto(&managedServe{
			desired:  serve,
			state:    networkpb.RuntimeState_RUNTIME_STATE_CONFIGURED,
			tunnelID: serve.TunnelID,
		}))
	}
	return resp, nil
}

func (a *Agent) ListConnects(_ context.Context, _ *networkpb.ListConnectsRequest) (*networkpb.ListConnectsResponse, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	resp := &networkpb.ListConnectsResponse{
		Connects: make([]*networkpb.ManagedConnectStatus, 0, len(a.network.Connects)),
	}
	for _, connect := range a.network.Connects {
		if _, actor := a.findConnectActorLocked(connect.TunnelID, connect.Name, connect.ListenAddress); actor != nil {
			resp.Connects = append(resp.Connects, connectStatusProto(actor))
			continue
		}
		resp.Connects = append(resp.Connects, connectStatusProto(&managedConnect{
			desired:  connect,
			state:    networkpb.RuntimeState_RUNTIME_STATE_CONFIGURED,
			tunnelID: connect.TunnelID,
		}))
	}
	return resp, nil
}
