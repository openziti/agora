package main

import (
	"context"
	"fmt"

	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/sdk/agent/networkpb"
	"github.com/spf13/cobra"
)

func init() {
	tunnelCmd.AddCommand(newTunnelServeCommand().cmd)
}

type tunnelServeCommand struct {
	mode        string
	backend     string
	foreground  bool
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
	cmd.Flags().BoolVar(&command.foreground, "foreground", false, "Run the tunnel runtime directly instead of using the local network agent")
	cmd.Flags().StringSliceVar(&command.grantEmails, "grant", nil, "Grant access to an account email (repeatable)")
	panicIfErr(cmd.MarkFlagRequired("mode"))
	panicIfErr(cmd.MarkFlagRequired("backend"))
	cmd.RunE = command.runE
	return command
}

func (cmd *tunnelServeCommand) runE(_ *cobra.Command, args []string) (err error) {
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

func (cmd *tunnelServeCommand) run(args []string) {
	panicIfUnsupportedMode(cmd.mode)
	mode := api.TunnelMode(cmd.mode)
	panicIfErr(validateModeAndTarget(mode, cmd.backend))

	if cmd.foreground {
		cmd.runForeground(args)
		return
	}

	root := requireEnabledRoot()
	client, cleanup := requireRunningNetworkClient(root)
	defer cleanup()

	res, err := client.EnsureServe(context.Background(), &networkpb.EnsureServeRequest{
		Name:          args[0],
		Mode:          cmd.mode,
		BackendTarget: cmd.backend,
		GrantEmails:   append([]string(nil), cmd.grantEmails...),
	})
	panicIfErr(err)

	fmt.Printf(
		"serving tunnel '%s' state='%s' tunnel_id='%s' serve_id='%s'\n",
		res.Serve.Desired.Name,
		formatRuntimeState(res.Serve.State),
		res.Serve.TunnelId,
		res.Serve.ServeId,
	)
}

func (cmd *tunnelServeCommand) runForeground(args []string) {
	mode := api.TunnelMode(cmd.mode)
	root := requireEnabledRoot()
	env := root.Environment()
	client := openEnvironmentAPIClient(root)

	tunnel := ensureTunnelCreatedOrReused(client, &api.CreateTunnelRequest{
		EnvironmentId: env.EnvironmentID,
		Name:          args[0],
		Mode:          mode,
		BackendTarget: api.NewOptString(cmd.backend),
		GrantEmails:   cmd.grantEmails,
	}, env.EnvironmentID)
	for _, email := range cmd.grantEmails {
		ignoreConflictGrantAdd(client, tunnel.ID, email)
	}

	ctx, cancel := signalContext()
	defer cancel()

	serveRes, err := client.StartTunnelServe(context.Background(), &api.StartTunnelServeRequest{
		EnvironmentId: env.EnvironmentID,
	}, api.StartTunnelServeParams{TunnelId: tunnel.ID})
	panicIfErr(err)

	serve, ok := serveRes.(*api.TunnelServe)
	if !ok {
		switch typed := serveRes.(type) {
		case *api.StartTunnelServeNotFound:
			panic(typed.Message)
		case *api.StartTunnelServeConflict:
			panic(typed.Message)
		case *api.StartTunnelServeUnauthorized:
			panic(typed.Message)
		case *api.StartTunnelServeInternalServerError:
			panic(typed.Message)
		default:
			panic(fmt.Sprintf("unexpected start tunnel serve response: %T", serveRes))
		}
	}
	defer cleanupTunnelServe(client, serve.ID)

	startTunnelServeHeartbeats(ctx, client, serve.ID)

	identityPath := requireEnvironmentIdentityPath(root)
	fmt.Printf("serving tunnel '%s' mode='%s' backend='%s'\n", tunnel.Name, tunnel.Mode, formatTunnelBackend(*tunnel))
	panicIfErr(runServeRuntime(ctx, identityPath, tunnel))
}
