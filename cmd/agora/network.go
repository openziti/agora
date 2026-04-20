package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/openziti/agora/environment"
	"github.com/openziti/agora/environment/env_core"
	networkagent "github.com/openziti/agora/internal/network/agent"
	networkpb "github.com/openziti/agora/internal/network/agent/pb"
	"github.com/spf13/cobra"
)

const networkDialTimeout = 2 * time.Second

var networkCmd = &cobra.Command{
	Use:   "network",
	Short: "Local Layer 1 network runtime commands",
}

func init() {
	rootCmd.AddCommand(networkCmd)
}

func loadRoot() env_core.Root {
	root, err := environment.LoadRoot()
	if err != nil {
		panic(err)
	}
	return root
}

func networkSocketPath(root env_core.Root) string {
	socketPath, err := root.NetworkSocketPath()
	if err != nil {
		panic(err)
	}
	return socketPath
}

func networkStatusOrNil(root env_core.Root) (status *networkpb.NetworkStatus, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), networkDialTimeout)
	defer cancel()
	status, err = networkagent.Status(ctx, networkSocketPath(root))
	if errors.Is(err, networkagent.ErrNotRunning) {
		return nil, nil
	}
	return status, err
}

func reloadNetworkAgentIfRunning(root env_core.Root) error {
	return networkagent.ReloadEnvironmentIfRunning(root)
}

func openNetworkClient(root env_core.Root) (networkpb.NetworkServiceClient, func(), error) {
	ctx, cancel := context.WithTimeout(context.Background(), networkDialTimeout)
	defer cancel()

	if _, err := networkagent.Ping(ctx, networkSocketPath(root)); err != nil {
		return nil, nil, err
	}

	conn, err := networkagent.Dial(ctx, networkSocketPath(root))
	if err != nil {
		return nil, nil, err
	}
	cleanup := func() { _ = conn.Close() }
	return networkpb.NewNetworkServiceClient(conn), cleanup, nil
}

func requireRunningNetworkClient(root env_core.Root) (networkpb.NetworkServiceClient, func()) {
	client, cleanup, err := openNetworkClient(root)
	if err != nil {
		if errors.Is(err, networkagent.ErrNotRunning) {
			panic("agora network is not running; run 'agora network start'")
		}
		panic(err)
	}
	return client, cleanup
}

func requireRunningNetworkClientError(root env_core.Root) (networkpb.NetworkServiceClient, func(), error) {
	client, cleanup, err := openNetworkClient(root)
	if err != nil {
		if errors.Is(err, networkagent.ErrNotRunning) {
			return nil, nil, fmt.Errorf("agora network is not running; run 'agora network start'")
		}
		return nil, nil, err
	}
	return client, cleanup, nil
}
