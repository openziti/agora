package main

import (
	"context"
	"errors"
	"fmt"

	networkagent "github.com/openziti/agora/internal/network/agent"
	"github.com/spf13/cobra"
)

func init() {
	networkCmd.AddCommand(newNetworkStopCommand())
}

func newNetworkStopCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "stop",
		Short: "Stop the local Layer 1 network runtime",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			root := loadRoot()
			ctx, cancel := context.WithTimeout(context.Background(), networkDialTimeout)
			defer cancel()

			if err := networkagent.Stop(ctx, networkSocketPath(root)); err != nil {
				if errors.Is(err, networkagent.ErrNotRunning) {
					fmt.Println("agora network is not running")
					return nil
				}
				return err
			}

			fmt.Println("stopped agora network")
			return nil
		},
	}
}
