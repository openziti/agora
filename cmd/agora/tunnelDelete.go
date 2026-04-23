package main

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/openziti/agora/environment/env_core"
	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/internal/network/daemon"
	"github.com/openziti/agora/sdk/agent/networkpb"
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
		Use:   "delete <name|tt_...|ts_...|ta_...>",
		Short: "Delete tunnel resources or managed runtime state",
		Args:  cobra.ExactArgs(1),
	}
	command := &tunnelDeleteCommand{cmd: cmd}
	cmd.Run = command.run
	cmd.AddCommand(newTunnelDeleteServeCommand().cmd)
	cmd.AddCommand(newTunnelDeleteConnectCommand().cmd)
	return command
}

func (cmd *tunnelDeleteCommand) run(_ *cobra.Command, args []string) {
	root := requireEnabledRoot()
	token := args[0]

	switch {
	case isTunnelServeID(token):
		client, cleanup := requireRunningNetworkClient(root)
		defer cleanup()
		_, err := client.RemoveServe(context.Background(), &networkpb.RemoveServeRequest{ServeId: token})
		panicIfErr(err)
		fmt.Printf("deleted managed serve '%s'\n", token)
		return
	case isTunnelAttachmentID(token):
		client, cleanup := requireRunningNetworkClient(root)
		defer cleanup()
		_, err := client.RemoveConnect(context.Background(), &networkpb.RemoveConnectRequest{AttachmentId: token})
		panicIfErr(err)
		fmt.Printf("deleted managed connect '%s'\n", token)
		return
	}

	client := openEnvironmentAPIClient(root)
	tunnel := resolveManagedTunnelIdentifier(client, token)
	drainManagedTunnelState(root, tunnel.ID, tunnel.Name)

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

type tunnelDeleteServeCommand struct {
	cmd *cobra.Command
}

func newTunnelDeleteServeCommand() *tunnelDeleteServeCommand {
	cmd := &cobra.Command{
		Use:   "serve <name|tt_...>",
		Short: "Delete managed serve state",
		Args:  cobra.ExactArgs(1),
	}
	command := &tunnelDeleteServeCommand{cmd: cmd}
	cmd.Run = command.run
	return command
}

func (cmd *tunnelDeleteServeCommand) run(_ *cobra.Command, args []string) {
	root := requireEnabledRoot()
	token := args[0]
	tunnelID := ""
	name := token
	if isTunnelID(token) {
		tunnelID = token
		name = ""
	}

	if client, cleanup, err := openNetworkClient(root); err == nil {
		defer cleanup()
		_, err = client.RemoveServe(context.Background(), &networkpb.RemoveServeRequest{
			TunnelId: tunnelID,
			Name:     name,
		})
		panicIfErr(err)
		fmt.Printf("deleted managed serve '%s'\n", token)
		return
	} else if err != nil && !isErrNotRunning(err) {
		panic(err)
	}

	removed, err := removeLocalDesiredServe(root, tunnelID, name)
	panicIfErr(err)
	if !removed {
		panic("managed serve not found")
	}
	fmt.Printf("deleted managed serve '%s'\n", token)
}

type tunnelDeleteConnectCommand struct {
	listen string
	cmd    *cobra.Command
}

func newTunnelDeleteConnectCommand() *tunnelDeleteConnectCommand {
	cmd := &cobra.Command{
		Use:   "connect <name|tt_...>",
		Short: "Delete managed connect state",
		Args:  cobra.ExactArgs(1),
	}
	command := &tunnelDeleteConnectCommand{cmd: cmd}
	cmd.Flags().StringVar(&command.listen, "listen", "", "Local listen address for the managed connect")
	panicIfErr(cmd.MarkFlagRequired("listen"))
	cmd.Run = command.run
	return command
}

func (cmd *tunnelDeleteConnectCommand) run(_ *cobra.Command, args []string) {
	panicIfErr(validateListenAddress(cmd.listen))

	root := requireEnabledRoot()
	token := args[0]
	tunnelID := ""
	name := token
	if isTunnelID(token) {
		tunnelID = token
		name = ""
	}

	if client, cleanup, err := openNetworkClient(root); err == nil {
		defer cleanup()
		_, err = client.RemoveConnect(context.Background(), &networkpb.RemoveConnectRequest{
			TunnelId:      tunnelID,
			Name:          name,
			ListenAddress: cmd.listen,
		})
		panicIfErr(err)
		fmt.Printf("deleted managed connect '%s' listen='%s'\n", token, cmd.listen)
		return
	} else if err != nil && !isErrNotRunning(err) {
		panic(err)
	}

	removed, err := removeLocalDesiredConnect(root, tunnelID, name, cmd.listen)
	panicIfErr(err)
	if !removed {
		panic("managed connect not found")
	}
	fmt.Printf("deleted managed connect '%s' listen='%s'\n", token, cmd.listen)
}

func drainManagedTunnelState(root env_core.Root, tunnelID, name string) {
	if client, cleanup, err := openNetworkClient(root); err == nil {
		defer cleanup()
		status, err := client.GetNetworkStatus(context.Background(), &networkpb.GetNetworkStatusRequest{})
		panicIfErr(err)
		for _, serve := range status.Status.Serves {
			if serve.TunnelId != tunnelID && !strings.EqualFold(serve.Desired.Name, name) {
				continue
			}
			_, err := client.RemoveServe(context.Background(), &networkpb.RemoveServeRequest{
				ServeId:  serve.ServeId,
				TunnelId: serve.TunnelId,
				Name:     serve.Desired.Name,
			})
			panicIfErr(err)
		}
		for _, connect := range status.Status.Connects {
			if connect.TunnelId != tunnelID && !strings.EqualFold(connect.Desired.Name, name) {
				continue
			}
			_, err := client.RemoveConnect(context.Background(), &networkpb.RemoveConnectRequest{
				AttachmentId:  connect.AttachmentId,
				TunnelId:      connect.TunnelId,
				Name:          connect.Desired.Name,
				ListenAddress: connect.Desired.ListenAddress,
			})
			panicIfErr(err)
		}
		return
	} else if err != nil && !isErrNotRunning(err) {
		panic(err)
	}

	panicIfErr(removeLocalDesiredTunnel(root, tunnelID, name))
}

func isErrNotRunning(err error) bool {
	return errors.Is(err, daemon.ErrNotRunning)
}
