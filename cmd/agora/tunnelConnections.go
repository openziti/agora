package main

import (
	"context"
	"fmt"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/internal/clioutput"
	"github.com/spf13/cobra"
)

func init() {
	tunnelCmd.AddCommand(newTunnelConnectionsCommand().cmd)
}

type tunnelConnectionsCommand struct {
	jsonOutput bool
	cmd        *cobra.Command
}

func newTunnelConnectionsCommand() *tunnelConnectionsCommand {
	cmd := &cobra.Command{
		Use:     "connections <name>",
		Aliases: []string{"attachments"},
		Short:   "List active and historical tunnel connections",
		Args:    cobra.ExactArgs(1),
	}
	command := &tunnelConnectionsCommand{cmd: cmd}
	cmd.Flags().BoolVar(&command.jsonOutput, "json", false, "Output raw JSON instead of a table")
	cmd.Run = command.run
	return command
}

func (cmd *tunnelConnectionsCommand) run(_ *cobra.Command, args []string) {
	root := requireEnabledRoot()
	client := openEnvironmentAPIClient(root)
	tunnel := mustResolveManagedTunnel(client, args[0])

	res, err := client.ListTunnelAttachments(context.Background(), api.ListTunnelAttachmentsParams{TunnelId: tunnel.ID})
	panicIfErr(err)

	attachments, ok := res.(*api.ListTunnelAttachmentsResponse)
	if !ok {
		switch typed := res.(type) {
		case *api.ListTunnelAttachmentsNotFound:
			panic(typed.Message)
		case *api.ListTunnelAttachmentsUnauthorized:
			panic(typed.Message)
		case *api.ListTunnelAttachmentsInternalServerError:
			panic(typed.Message)
		}
		panic(fmt.Sprintf("unexpected list tunnel attachments response: %T", res))
	}

	if cmd.jsonOutput {
		panicIfErr(clioutput.RenderJSON(attachments))
		return
	}

	t := clioutput.NewTable()
	t.AppendHeader(table.Row{"Account", "Environment ID", "Listen", "State", "Heartbeat", "Disconnected", "Created"})
	for _, attachment := range *attachments {
		t.AppendRow(table.Row{
			clioutput.StringOrDash(optionalString(attachment.AccountEmail)),
			attachment.EnvironmentId,
			attachment.ListenAddress,
			attachment.State,
			clioutput.TimeUTC(attachment.LastHeartbeatAt),
			optionalTime(attachment.DisconnectedAt),
			clioutput.TimeUTC(attachment.CreatedAt),
		})
	}
	clioutput.PrintTable(t)
	clioutput.PrintTotal("connection(s)", len(*attachments))
}
