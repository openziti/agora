package main

import (
	"context"
	"fmt"

	"github.com/openziti/agora/internal/api"
	"github.com/spf13/cobra"
)

func init() {
	tunnelCmd.AddCommand(newTunnelGrantCommand().cmd)
}

type tunnelGrantCommand struct {
	cmd *cobra.Command
}

func newTunnelGrantCommand() *tunnelGrantCommand {
	cmd := &cobra.Command{
		Use:   "grant <name> <email>",
		Short: "Grant another account access to a tunnel",
		Args:  cobra.ExactArgs(2),
	}
	command := &tunnelGrantCommand{cmd: cmd}
	cmd.Run = command.run
	return command
}

func (cmd *tunnelGrantCommand) run(_ *cobra.Command, args []string) {
	root := requireEnabledRoot()
	client := openEnvironmentAPIClient(root)
	tunnel := mustResolveManagedTunnel(client, args[0])

	res, err := client.AddTunnelGrant(context.Background(), &api.AddTunnelGrantRequest{Email: args[1]}, api.AddTunnelGrantParams{TunnelId: tunnel.ID})
	panicIfErr(err)

	switch typed := res.(type) {
	case *api.TunnelGrant:
		fmt.Printf("granted '%s' access to tunnel '%s'\n", typed.Email, tunnel.Name)
	case *api.AddTunnelGrantConflict:
		fmt.Printf("account '%s' already has access to tunnel '%s'\n", args[1], tunnel.Name)
	case *api.AddTunnelGrantNotFound:
		panic(typed.Message)
	case *api.AddTunnelGrantUnauthorized:
		panic(typed.Message)
	case *api.AddTunnelGrantInternalServerError:
		panic(typed.Message)
	default:
		panic(fmt.Sprintf("unexpected add tunnel grant response: %T", res))
	}
}
