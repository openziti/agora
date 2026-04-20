package main

import (
	"context"
	"fmt"
	"os"

	"github.com/openziti/agora/environment"
	"github.com/openziti/agora/environment/env_core"
	"github.com/openziti/agora/internal/api"
	networkpb "github.com/openziti/agora/internal/network/agent/pb"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(newDisableCommand().cmd)
}

type disableCommand struct {
	cmd *cobra.Command
}

func newDisableCommand() *disableCommand {
	cmd := &cobra.Command{
		Use:   "disable",
		Short: "Disable the local agora environment",
		Args:  cobra.NoArgs,
	}
	command := &disableCommand{cmd: cmd}
	cmd.Run = command.run
	return command
}

func (cmd *disableCommand) run(_ *cobra.Command, _ []string) {
	root, err := environment.LoadRoot()
	if err != nil {
		panic(err)
	}
	if !root.IsEnabled() {
		panic("no environment is enabled")
	}

	drainManagedRuntimeForDisable(root)

	env := root.Environment()
	client := openAccountAPIClient(env.APIEndpoint, env.AccountToken)
	res, err := client.DisableEnvironment(context.Background(), api.DisableEnvironmentParams{EnvironmentId: env.EnvironmentID})
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: controller cleanup failed: %v\n", err)
	} else if _, ok := res.(*api.DisableEnvironmentNoContent); !ok {
		fmt.Fprintf(os.Stderr, "warning: controller cleanup returned %T; removing local environment anyway\n", res)
	}

	if err := root.DeleteEnvironment(); err != nil {
		panic(err)
	}
	if err := root.DeleteZitiIdentityNamed(environmentIdentityName); err != nil {
		panic(err)
	}
	if err := root.DeleteNetwork(); err != nil {
		panic(err)
	}
	if err := reloadNetworkAgentIfRunning(root); err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to reload agora network runtime: %v\n", err)
	}

	fmt.Println("disabled local environment")
}

func drainManagedRuntimeForDisable(root env_core.Root) {
	client, cleanup, err := openNetworkClient(root)
	if err != nil {
		if !isErrNotRunning(err) {
			fmt.Fprintf(os.Stderr, "warning: failed to connect to agora network runtime: %v\n", err)
		}
		return
	}
	defer cleanup()

	status, err := client.GetNetworkStatus(context.Background(), &networkpb.GetNetworkStatusRequest{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to query agora network runtime: %v\n", err)
		return
	}

	for _, serve := range status.Status.Serves {
		_, err := client.RemoveServe(context.Background(), &networkpb.RemoveServeRequest{
			ServeId:  serve.ServeId,
			TunnelId: serve.TunnelId,
			Name:     serve.Desired.Name,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to remove managed serve '%s': %v\n", serve.Desired.Name, err)
		}
	}
	for _, connect := range status.Status.Connects {
		_, err := client.RemoveConnect(context.Background(), &networkpb.RemoveConnectRequest{
			AttachmentId:  connect.AttachmentId,
			TunnelId:      connect.TunnelId,
			Name:          connect.Desired.Name,
			ListenAddress: connect.Desired.ListenAddress,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to remove managed connect '%s' listen='%s': %v\n", connect.Desired.Name, connect.Desired.ListenAddress, err)
		}
	}
}
