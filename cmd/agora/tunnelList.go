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
	tunnelCmd.AddCommand(newTunnelListCommand().cmd)
}

type tunnelListCommand struct {
	jsonOutput bool
	accessible bool
	all        bool
	cmd        *cobra.Command
}

func newTunnelListCommand() *tunnelListCommand {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List tunnels visible to the current account",
		Args:  cobra.NoArgs,
	}
	command := &tunnelListCommand{cmd: cmd}
	cmd.Flags().BoolVar(&command.jsonOutput, "json", false, "Output raw JSON instead of a table")
	cmd.Flags().BoolVar(&command.accessible, "accessible", false, "List granted tunnels owned by other accounts")
	cmd.Flags().BoolVar(&command.all, "all", false, "List all tunnels visible to the current account")
	cmd.Run = command.run
	return command
}

func (cmd *tunnelListCommand) run(_ *cobra.Command, _ []string) {
	if cmd.accessible && cmd.all {
		panic("only one of '--accessible' or '--all' may be used")
	}

	scope := api.ListTunnelsScopeOwned
	switch {
	case cmd.all:
		scope = api.ListTunnelsScopeAll
	case cmd.accessible:
		scope = api.ListTunnelsScopeAccessible
	}

	root := requireEnabledRoot()
	client := openEnvironmentAPIClient(root)

	params := api.ListTunnelsParams{}
	params.Scope.SetTo(scope)

	res, err := client.ListTunnels(context.Background(), params)
	panicIfErr(err)

	tunnels, ok := res.(*api.ListTunnelsResponse)
	if !ok {
		switch typed := res.(type) {
		case *api.ListTunnelsUnauthorized:
			panic(typed.Message)
		case *api.ListTunnelsInternalServerError:
			panic(typed.Message)
		}
		panic(fmt.Sprintf("unexpected list tunnels response: %T", res))
	}

	if cmd.jsonOutput {
		panicIfErr(clioutput.RenderJSON(tunnels))
		return
	}

	t := clioutput.NewTable()
	t.AppendHeader(table.Row{"Name", "Mode", "Backend", "State", "Tunnel ID", "Environment ID"})
	for _, tunnel := range *tunnels {
		t.AppendRow(table.Row{
			tunnel.Name,
			tunnel.Mode,
			tunnel.BackendTarget,
			tunnel.State,
			tunnel.ID,
			tunnel.EnvironmentId,
		})
	}
	clioutput.PrintTable(t)
	clioutput.PrintTotal("tunnel(s)", len(*tunnels))
}
