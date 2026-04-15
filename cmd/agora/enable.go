package main

import (
	"context"
	"fmt"
	"os"

	"github.com/openziti/agora/environment"
	"github.com/openziti/agora/environment/env_core"
	"github.com/openziti/agora/internal/api"
	"github.com/spf13/cobra"
)

const environmentIdentityName = "environment"

func init() {
	rootCmd.AddCommand(newEnableCommand().cmd)
}

type enableCommand struct {
	description string
	host        string
	cmd         *cobra.Command
}

func newEnableCommand() *enableCommand {
	cmd := &cobra.Command{
		Use:   "enable <account-token>",
		Short: "Enable a local agora environment",
		Args:  cobra.ExactArgs(1),
	}
	command := &enableCommand{cmd: cmd}
	cmd.Flags().StringVar(&command.description, "description", "", "Optional environment description")
	cmd.Flags().StringVar(&command.host, "host", "", "Optional host value to report for this environment")
	cmd.Run = command.run
	return command
}

func (cmd *enableCommand) run(_ *cobra.Command, args []string) {
	root, err := environment.LoadRoot()
	if err != nil {
		panic(err)
	}
	if root.IsEnabled() {
		panic("environment is already enabled; run 'agora disable' first")
	}

	apiEndpoint, _ := root.APIEndpoint()
	if apiEndpoint == "" {
		panic("api endpoint is not configured; set AGORA_API_ENDPOINT or run 'agora config set api_endpoint <url>'")
	}

	host := cmd.host
	if host == "" {
		host, err = os.Hostname()
		if err != nil {
			panic(err)
		}
	}

	description := cmd.description
	if description == "" {
		description = host
	}

	client := openAccountAPIClient(apiEndpoint, args[0])
	req := &api.EnableEnvironmentRequest{}
	req.Description.SetTo(description)
	req.Host.SetTo(host)

	res, err := client.EnableEnvironment(context.Background(), req)
	if err != nil {
		panic(err)
	}

	enabled, ok := res.(*api.EnableEnvironmentResponse)
	if !ok {
		panic(fmt.Sprintf("enable environment failed: %T", res))
	}

	if err := root.SetEnvironment(&env_core.Environment{
		EnvironmentID: enabled.Environment.ID,
		AccountToken:  args[0],
		ZitiIdentity:  enabled.Environment.ZitiIdentityId,
		APIEndpoint:   apiEndpoint,
	}); err != nil {
		panic(err)
	}
	if err := root.SaveZitiIdentityNamed(environmentIdentityName, enabled.EnrollmentJson); err != nil {
		_ = root.DeleteEnvironment()
		panic(err)
	}
	if err := reloadNetworkAgentIfRunning(root); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "warning: failed to reload agora network runtime: %v\n", err)
	}

	fmt.Printf("enabled environment '%s'\n", enabled.Environment.ID)
}
