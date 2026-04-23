package main

import (
	"errors"
	"fmt"

	"github.com/michaelquigley/df/dl"
	"github.com/openziti/agora/sdk/agent"
	"github.com/spf13/cobra"
)

func init() {
	networkCmd.AddCommand(newNetworkStartCommand())
}

func newNetworkStartCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "start",
		Short: "Run the local Layer 1 network runtime in the foreground",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			runtime, err := agent.NewDaemon()
			if err != nil {
				return err
			}

			ctx, cancel := signalContext()
			defer cancel()

			if err := runtime.Run(ctx); err != nil {
				if errors.Is(err, agent.ErrAlreadyRunning) {
					fmt.Printf("agora network is already running socket='%s'\n", networkSocketPath(loadRoot()))
					return nil
				}
				return err
			}

			dl.Debug("agora network exited")
			return nil
		},
	}
}
