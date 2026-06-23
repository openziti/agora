package main

import (
	"context"
	"fmt"

	"github.com/openziti/agora/internal/api"
	"github.com/spf13/cobra"
)

func init() {
	tunnelCmd.AddCommand(newTunnelTakeoverCommand().cmd)
}

type tunnelTakeoverCommand struct {
	cmd *cobra.Command
}

func newTunnelTakeoverCommand() *tunnelTakeoverCommand {
	cmd := &cobra.Command{
		Use:   "takeover <name|tt_...>",
		Short: "Reclaim a tunnel by evicting whatever is currently hosting it",
		Long: "Evict whatever is currently hosting the tunnel -- deleting its terminators on the " +
			"fabric and clearing the serve record for a proxy tunnel -- so another environment can take " +
			"the spot. Takeover is unconditional: it displaces whatever it finds without checking the " +
			"current host's health. Use 'tunnel serve --takeover' to take over and serve in one step; " +
			"this command is how a direct tunnel (hosted by the SDK runtime) is reclaimed.",
		Args: cobra.ExactArgs(1),
	}
	command := &tunnelTakeoverCommand{cmd: cmd}
	cmd.Run = command.run
	return command
}

func (cmd *tunnelTakeoverCommand) run(_ *cobra.Command, args []string) {
	root := requireEnabledRoot()
	client := openEnvironmentAPIClient(root)
	tunnel := resolveManagedTunnelIdentifier(client, args[0])
	takeoverManagedTunnel(client, tunnel)
}

// takeoverManagedTunnel calls the controller's takeover operation for the given tunnel, reporting the
// outcome. It panics on error or a non-success response (the CLI's recover wrapper turns that into an
// error result).
func takeoverManagedTunnel(client *api.Client, tunnel *api.Tunnel) {
	res, err := client.TakeoverTunnel(context.Background(), api.TakeoverTunnelParams{TunnelId: tunnel.ID})
	panicIfErr(err)
	switch typed := res.(type) {
	case *api.TakeoverTunnelNoContent:
		fmt.Printf("took over tunnel '%s'\n", tunnel.Name)
	case *api.TakeoverTunnelNotFound:
		panic(typed.Message)
	case *api.TakeoverTunnelConflict:
		panic(typed.Message)
	case *api.TakeoverTunnelUnauthorized:
		panic(typed.Message)
	case *api.TakeoverTunnelInternalServerError:
		panic(typed.Message)
	default:
		panic(fmt.Sprintf("unexpected takeover tunnel response: %T", res))
	}
}
