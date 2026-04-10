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
	tunnelCmd.AddCommand(newTunnelGrantsCommand().cmd)
}

type tunnelGrantsCommand struct {
	jsonOutput bool
	cmd        *cobra.Command
}

func newTunnelGrantsCommand() *tunnelGrantsCommand {
	cmd := &cobra.Command{
		Use:   "grants <name>",
		Short: "List tunnel grants",
		Args:  cobra.ExactArgs(1),
	}
	command := &tunnelGrantsCommand{cmd: cmd}
	cmd.Flags().BoolVar(&command.jsonOutput, "json", false, "Output raw JSON instead of a table")
	cmd.Run = command.run
	return command
}

func (cmd *tunnelGrantsCommand) run(_ *cobra.Command, args []string) {
	root := requireEnabledRoot()
	client := openEnvironmentAPIClient(root)
	tunnel := mustResolveManagedTunnel(client, args[0])

	res, err := client.ListTunnelGrants(context.Background(), api.ListTunnelGrantsParams{TunnelId: tunnel.ID})
	panicIfErr(err)

	grants, ok := res.(*api.ListTunnelGrantsResponse)
	if !ok {
		switch typed := res.(type) {
		case *api.ListTunnelGrantsNotFound:
			panic(typed.Message)
		case *api.ListTunnelGrantsUnauthorized:
			panic(typed.Message)
		case *api.ListTunnelGrantsInternalServerError:
			panic(typed.Message)
		}
		panic(fmt.Sprintf("unexpected list tunnel grants response: %T", res))
	}

	if cmd.jsonOutput {
		panicIfErr(clioutput.RenderJSON(grants))
		return
	}

	t := clioutput.NewTable()
	t.AppendHeader(table.Row{"Email", "Account ID", "Created"})
	for _, grant := range *grants {
		t.AppendRow(table.Row{
			grant.Email,
			grant.AccountId,
			clioutput.TimeUTC(grant.CreatedAt),
		})
	}
	clioutput.PrintTable(t)
	clioutput.PrintTotal("grant(s)", len(*grants))
}
