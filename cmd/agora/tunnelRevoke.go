package main

import (
	"context"
	"fmt"

	"github.com/openziti/agora/internal/api"
	"github.com/spf13/cobra"
)

func init() {
	tunnelCmd.AddCommand(newTunnelRevokeCommand().cmd)
}

type tunnelRevokeCommand struct {
	cmd *cobra.Command
}

func newTunnelRevokeCommand() *tunnelRevokeCommand {
	cmd := &cobra.Command{
		Use:   "revoke <name> <email>",
		Short: "Revoke another account's access to a tunnel",
		Args:  cobra.ExactArgs(2),
	}
	command := &tunnelRevokeCommand{cmd: cmd}
	cmd.Run = command.run
	return command
}

func (cmd *tunnelRevokeCommand) run(_ *cobra.Command, args []string) {
	root := requireEnabledRoot()
	client := openEnvironmentAPIClient(root)
	tunnel := mustResolveManagedTunnel(client, args[0])
	grants := listTunnelGrantsOrPanic(client, tunnel.ID)
	grant := findGrantByEmail(grants, args[1])
	if grant == nil {
		panic(fmt.Sprintf("account '%s' is not granted access to tunnel '%s'", args[1], tunnel.Name))
	}

	res, err := client.RemoveTunnelGrant(context.Background(), api.RemoveTunnelGrantParams{TunnelId: tunnel.ID, AccountId: grant.AccountId})
	panicIfErr(err)
	switch typed := res.(type) {
	case *api.RemoveTunnelGrantNoContent:
		fmt.Printf("revoked '%s' access to tunnel '%s'\n", args[1], tunnel.Name)
	case *api.RemoveTunnelGrantNotFound:
		panic(typed.Message)
	case *api.RemoveTunnelGrantUnauthorized:
		panic(typed.Message)
	case *api.RemoveTunnelGrantInternalServerError:
		panic(typed.Message)
	default:
		panic(fmt.Sprintf("unexpected remove tunnel grant response: %T", res))
	}
}
