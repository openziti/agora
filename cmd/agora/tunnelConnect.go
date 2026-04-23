package main

import (
	"context"
	"fmt"

	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/sdk/agent/networkpb"
	"github.com/spf13/cobra"
)

func init() {
	tunnelCmd.AddCommand(newTunnelConnectCommand().cmd)
}

type tunnelConnectCommand struct {
	listen     string
	foreground bool
	cmd        *cobra.Command
}

func newTunnelConnectCommand() *tunnelConnectCommand {
	cmd := &cobra.Command{
		Use:   "connect <name>",
		Short: "Connect to a tunnel from the local environment",
		Args:  cobra.ExactArgs(1),
	}
	command := &tunnelConnectCommand{cmd: cmd}
	cmd.Flags().StringVar(&command.listen, "listen", "", "Local listen address for the consumer side")
	cmd.Flags().BoolVar(&command.foreground, "foreground", false, "Run the tunnel runtime directly instead of using the local network agent")
	panicIfErr(cmd.MarkFlagRequired("listen"))
	cmd.RunE = command.runE
	return command
}

func (cmd *tunnelConnectCommand) runE(_ *cobra.Command, args []string) (err error) {
	defer func() {
		recovered := recover()
		if recovered == nil {
			return
		}
		if panicInstead {
			panic(recovered)
		}
		switch typed := recovered.(type) {
		case error:
			err = typed
		default:
			err = fmt.Errorf("%v", typed)
		}
	}()

	cmd.run(args)
	return nil
}

func (cmd *tunnelConnectCommand) run(args []string) {
	panicIfErr(validateListenAddress(cmd.listen))

	if cmd.foreground {
		cmd.runForeground(args)
		return
	}

	root := requireEnabledRoot()
	client, cleanup := requireRunningNetworkClient(root)
	defer cleanup()

	res, err := client.EnsureConnect(context.Background(), &networkpb.EnsureConnectRequest{
		Name:          args[0],
		ListenAddress: cmd.listen,
	})
	panicIfErr(err)

	fmt.Printf(
		"connected tunnel '%s' state='%s' tunnel_id='%s' attachment_id='%s'\n",
		res.Connect.Desired.Name,
		formatRuntimeState(res.Connect.State),
		res.Connect.TunnelId,
		res.Connect.AttachmentId,
	)
}

func (cmd *tunnelConnectCommand) runForeground(args []string) {
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
