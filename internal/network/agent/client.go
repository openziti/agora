package agent

import (
	"context"
	"errors"
	"os"
	"syscall"
	"time"

	"github.com/openziti/agora/environment/env_core"
	networkpb "github.com/openziti/agora/internal/network/agent/pb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

const defaultDialTimeout = 2 * time.Second

var ErrNotRunning = errors.New("agora network is not running")

func Dial(_ context.Context, socketPath string) (*grpc.ClientConn, error) {
	conn, err := grpc.NewClient(
		"unix://"+socketPath,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, normalizeDialError(err)
	}
	return conn, nil
}

func Ping(ctx context.Context, socketPath string) (*networkpb.PingResponse, error) {
	conn, err := Dial(ctx, socketPath)
	if err != nil {
		return nil, err
	}
	defer func() { _ = conn.Close() }()

	client := networkpb.NewNetworkServiceClient(conn)
	resp, err := client.Ping(ctx, &networkpb.PingRequest{})
	if err != nil {
		return nil, normalizeDialError(err)
	}
	return resp, nil
}

func Stop(ctx context.Context, socketPath string) error {
	conn, err := Dial(ctx, socketPath)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()

	client := networkpb.NewNetworkServiceClient(conn)
	_, err = client.Shutdown(ctx, &networkpb.ShutdownRequest{})
	if err != nil {
		return normalizeDialError(err)
	}
	return nil
}

func Status(ctx context.Context, socketPath string) (*networkpb.NetworkStatus, error) {
	conn, err := Dial(ctx, socketPath)
	if err != nil {
		return nil, err
	}
	defer func() { _ = conn.Close() }()

	client := networkpb.NewNetworkServiceClient(conn)
	resp, err := client.GetNetworkStatus(ctx, &networkpb.GetNetworkStatusRequest{})
	if err != nil {
		return nil, normalizeDialError(err)
	}
	return resp.Status, nil
}

func ReloadEnvironmentIfRunning(root env_core.Root) error {
	socketPath, err := root.NetworkSocketPath()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), defaultDialTimeout)
	defer cancel()

	conn, err := Dial(ctx, socketPath)
	if err != nil {
		if errors.Is(err, ErrNotRunning) {
			return nil
		}
		return err
	}
	defer func() { _ = conn.Close() }()

	client := networkpb.NewNetworkServiceClient(conn)
	_, err = client.ReloadEnvironment(ctx, &networkpb.ReloadEnvironmentRequest{})
	if err != nil {
		return normalizeDialError(err)
	}
	return nil
}

func normalizeDialError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return ErrNotRunning
	}
	switch status.Code(err) {
	case codes.Canceled, codes.DeadlineExceeded, codes.Unavailable:
		return ErrNotRunning
	}
	if errors.Is(err, os.ErrNotExist) {
		return ErrNotRunning
	}
	if errors.Is(err, syscall.ENOENT) || errors.Is(err, syscall.ECONNREFUSED) {
		return ErrNotRunning
	}
	return err
}
