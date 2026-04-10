package main

import (
	"context"
	"fmt"

	"github.com/openziti/agora/internal/api"
	"github.com/spf13/cobra"
)

func init() {
	tunnelCmd.AddCommand(newTunnelDeleteCommand().cmd)
}

type tunnelDeleteCommand struct {
	cmd *cobra.Command
}

func newTunnelDeleteCommand() *tunnelDeleteCommand {
	cmd := &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete a tunnel",
		Args:  cobra.ExactArgs(1),
	}
	command := &tunnelDeleteCommand{cmd: cmd}
	cmd.Run = command.run
	return command
}

func (cmd *tunnelDeleteCommand) run(_ *cobra.Command, args []string) {
	root := requireEnabledRoot()
	client := openEnvironmentAPIClient(root)
	tunnel := mustResolveManagedTunnel(client, args[0])

	res, err := client.DeleteTunnel(context.Background(), api.DeleteTunnelParams{TunnelId: tunnel.ID})
	panicIfErr(err)
	switch typed := res.(type) {
	case *api.DeleteTunnelNoContent:
		fmt.Printf("deleted tunnel '%s'\n", tunnel.Name)
	case *api.DeleteTunnelNotFound:
		panic(typed.Message)
	case *api.DeleteTunnelUnauthorized:
		panic(typed.Message)
	case *api.DeleteTunnelInternalServerError:
		panic(typed.Message)
	default:
		panic(fmt.Sprintf("unexpected delete tunnel response: %T", res))
	}
}
