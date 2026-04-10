package main

import (
	"fmt"

	"github.com/openziti/agora/internal/api"
	"github.com/spf13/cobra"
)

func init() {
	tunnelCmd.AddCommand(newTunnelServeCommand().cmd)
}

type tunnelServeCommand struct {
	mode        string
	backend     string
	grantEmails []string
	cmd         *cobra.Command
}

func newTunnelServeCommand() *tunnelServeCommand {
	cmd := &cobra.Command{
		Use:   "serve <name>",
		Short: "Provision and serve a tunnel from the local environment",
		Args:  cobra.ExactArgs(1),
	}
	command := &tunnelServeCommand{cmd: cmd}
	cmd.Flags().StringVar(&command.mode, "mode", "", "Tunnel mode: http, tcp, or udp")
	cmd.Flags().StringVar(&command.backend, "backend", "", "Backend target for the local provider")
	cmd.Flags().StringSliceVar(&command.grantEmails, "grant", nil, "Grant access to an account email (repeatable)")
	panicIfErr(cmd.MarkFlagRequired("mode"))
	panicIfErr(cmd.MarkFlagRequired("backend"))
	cmd.Run = command.run
	return command
}

func (cmd *tunnelServeCommand) run(_ *cobra.Command, args []string) {
	panicIfUnsupportedMode(cmd.mode)
	mode := api.TunnelMode(cmd.mode)
	panicIfErr(validateModeAndTarget(mode, cmd.backend))

	root := requireEnabledRoot()
	env := root.Environment()
	client := openEnvironmentAPIClient(root)

	tunnel := ensureTunnelCreatedOrReused(client, &api.CreateTunnelRequest{
		EnvironmentId: env.EnvironmentID,
		Name:          args[0],
		Mode:          mode,
		BackendTarget: cmd.backend,
		GrantEmails:   cmd.grantEmails,
	}, env.EnvironmentID)
	for _, email := range cmd.grantEmails {
		ignoreConflictGrantAdd(client, tunnel.ID, email)
	}

	ctx, cancel := signalContext()
	defer cancel()

	identityPath := requireEnvironmentIdentityPath(root)
	fmt.Printf("serving tunnel '%s' mode='%s' backend='%s'\n", tunnel.Name, tunnel.Mode, tunnel.BackendTarget)
	panicIfErr(runServeRuntime(ctx, identityPath, tunnel))
}
