package main

import (
	"context"
	"errors"
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
