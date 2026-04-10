package main

import (
	"context"
	"fmt"

	"github.com/openziti/agora/internal/api"
	"github.com/spf13/cobra"
)

func init() {
	tunnelCmd.AddCommand(newTunnelConnectCommand().cmd)
}

type tunnelConnectCommand struct {
	listen string
	cmd    *cobra.Command
}

func newTunnelConnectCommand() *tunnelConnectCommand {
	cmd := &cobra.Command{
		Use:   "connect <name>",
		Short: "Connect to a tunnel from the local environment",
		Args:  cobra.ExactArgs(1),
	}
	command := &tunnelConnectCommand{cmd: cmd}
	cmd.Flags().StringVar(&command.listen, "listen", "", "Local listen address for the consumer side")
	panicIfErr(cmd.MarkFlagRequired("listen"))
	cmd.Run = command.run
	return command
}

func (cmd *tunnelConnectCommand) run(_ *cobra.Command, args []string) {
	panicIfErr(validateListenAddress(cmd.listen))

	root := requireEnabledRoot()
	env := root.Environment()
	client := openEnvironmentAPIClient(root)

	res, err := client.ConnectTunnel(context.Background(), &api.ConnectTunnelRequest{
		EnvironmentId: env.EnvironmentID,
		Name:          args[0],
		ListenAddress: cmd.listen,
	})
	panicIfErr(err)

	connect, ok := res.(*api.ConnectTunnelResponse)
	if !ok {
		switch typed := res.(type) {
		case *api.ConnectTunnelNotFound:
			panic(typed.Message)
		case *api.ConnectTunnelConflict:
			panic(typed.Message)
		case *api.ConnectTunnelUnauthorized:
			panic(typed.Message)
		case *api.ConnectTunnelInternalServerError:
			panic(typed.Message)
		default:
			panic(fmt.Sprintf("unexpected connect tunnel response: %T", res))
		}
	}

	ctx, cancel := signalContext()
	defer cancel()
	defer cleanupAttachment(client, connect.Attachment.ID)

	startAttachmentHeartbeats(ctx, client, connect.Attachment.ID)

	identityPath := requireEnvironmentIdentityPath(root)
	fmt.Printf("connected tunnel '%s' mode='%s' listen='%s'\n", connect.Tunnel.Name, connect.Tunnel.Mode, connect.Attachment.ListenAddress)
	panicIfErr(runConnectRuntime(ctx, identityPath, &connect.Tunnel, connect.Attachment.ListenAddress))
}
