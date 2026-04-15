package main

import (
	"context"
	"fmt"
	"os"

	"github.com/openziti/agora/environment"
	"github.com/openziti/agora/internal/api"
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
	if err := reloadNetworkAgentIfRunning(root); err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to reload agora network runtime: %v\n", err)
	}

	fmt.Println("disabled local environment")
}
