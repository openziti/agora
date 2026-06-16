package main

import (
	"context"
	"fmt"

	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/internal/clioutput"
	"github.com/spf13/cobra"
)

func init() {
	tunnelCmd.AddCommand(newTunnelCreateCommand().cmd)
}

type tunnelCreateCommand struct {
	mode        string
	backend     string
	grantEmails []string
	jsonOutput  bool
	cmd         *cobra.Command
}

func newTunnelCreateCommand() *tunnelCreateCommand {
	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create a tunnel resource without serving it",
		Long: "Create a durable tunnel resource without serving or connecting to it. " +
			"Omit --backend to create a direct tunnel (served by an application via the SDK); " +
			"pass --backend to create a proxy tunnel that 'agora tunnel serve' can run later.",
		Args: cobra.ExactArgs(1),
	}
	command := &tunnelCreateCommand{cmd: cmd}
	cmd.Flags().StringVar(&command.mode, "mode", "", "Tunnel mode: http, tcp, or udp")
	cmd.Flags().StringVar(&command.backend, "backend", "", "Backend target; when set the tunnel is a proxy tunnel, otherwise it is direct")
	cmd.Flags().StringSliceVar(&command.grantEmails, "grant", nil, "Grant access to an account email (repeatable)")
	cmd.Flags().BoolVar(&command.jsonOutput, "json", false, "Output the raw tunnel resource as JSON")
	panicIfErr(cmd.MarkFlagRequired("mode"))
	cmd.RunE = command.runE
	return command
}

func (cmd *tunnelCreateCommand) runE(_ *cobra.Command, args []string) (err error) {
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

func (cmd *tunnelCreateCommand) run(args []string) {
	panicIfUnsupportedMode(cmd.mode)
	mode := api.TunnelMode(cmd.mode)

	root := requireEnabledRoot()
	env := root.Environment()
	client := openEnvironmentAPIClient(root)

	req := &api.CreateTunnelRequest{
		EnvironmentId: env.EnvironmentID,
		Name:          args[0],
		Mode:          mode,
		GrantEmails:   cmd.grantEmails,
	}
	if cmd.backend != "" {
		panicIfErr(validateModeAndTarget(mode, cmd.backend))
		req.BackendTarget = api.NewOptString(cmd.backend)
	}

	res, err := client.CreateTunnel(context.Background(), req)
	panicIfErr(err)
	tunnel, err := tunnelFromCreateResult(res)
	panicIfErr(err)

	if cmd.jsonOutput {
		panicIfErr(clioutput.RenderJSON(tunnel))
		return
	}

	fmt.Printf(
		"created tunnel '%s' id='%s' mode='%s' kind='%s' backend='%s'\n",
		tunnel.Name,
		tunnel.ID,
		tunnel.Mode,
		tunnel.Kind,
		formatTunnelBackend(*tunnel),
	)
}

// tunnelFromCreateResult maps a create tunnel response variant to the created
// tunnel or a descriptive error. Unlike the serve path it does not reuse an
// existing tunnel: a name conflict is surfaced as an error.
func tunnelFromCreateResult(res api.CreateTunnelRes) (*api.Tunnel, error) {
	switch typed := res.(type) {
	case *api.Tunnel:
		return typed, nil
	case *api.CreateTunnelConflict:
		return nil, fmt.Errorf("create tunnel failed: %s", typed.Message)
	case *api.CreateTunnelNotFound:
		return nil, fmt.Errorf("create tunnel failed: %s", typed.Message)
	case *api.CreateTunnelUnauthorized:
		return nil, fmt.Errorf("create tunnel failed: %s", typed.Message)
	case *api.CreateTunnelInternalServerError:
		return nil, fmt.Errorf("create tunnel failed: %s", typed.Message)
	default:
		return nil, fmt.Errorf("unexpected create tunnel response: %T", res)
	}
}
