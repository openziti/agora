package agent

import (
	"context"
	"time"

	"github.com/openziti/agora/sdk/agent/networkpb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Private helpers used by the in-package daemon lifecycle
// (NewDaemon, (*Runtime).Run, prepareSocket) to check for an
// already-running daemon before binding the UDS. Embedded callers
// never touch this path. When the daemon lifecycle is eventually
// extracted to internal/network/daemon as its own package, these
// move with it and this file is deleted.

const defaultDialTimeout = 2 * time.Second

func dialUDS(socketPath string) (*grpc.ClientConn, error) {
	return grpc.NewClient(
		"unix://"+socketPath,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
}

func ping(ctx context.Context, socketPath string) (*networkpb.PingResponse, error) {
	conn, err := dialUDS(socketPath)
	if err != nil {
		return nil, err
	}
	defer func() { _ = conn.Close() }()
	client := networkpb.NewNetworkServiceClient(conn)
	return client.Ping(ctx, &networkpb.PingRequest{})
}
