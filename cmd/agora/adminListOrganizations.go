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
	adminListCmd.AddCommand(newAdminListOrganizationsCommand().cmd)
}

type adminListOrganizationsCommand struct {
	cmd        *cobra.Command
	jsonOutput bool
}

func newAdminListOrganizationsCommand() *adminListOrganizationsCommand {
	cmd := &cobra.Command{
		Use:     "organizations",
		Aliases: []string{"orgs"},
		Short:   "List organizations",
		Args:    cobra.NoArgs,
	}
	command := &adminListOrganizationsCommand{cmd: cmd}
	cmd.Flags().BoolVar(&command.jsonOutput, "json", false, "Output raw JSON instead of a table")
	cmd.Run = command.run
	return command
}

func (cmd *adminListOrganizationsCommand) run(_ *cobra.Command, _ []string) {
	client := openAdminAPIClient()

	res, err := client.ListOrganizations(context.Background())
	if err != nil {
		panic(err)
	}
	organizations, ok := res.(*api.ListOrganizationsResponse)
	if !ok {
		panic(fmt.Sprintf("unexpected list organizations response: %T", res))
	}

	if cmd.jsonOutput {
		if err := clioutput.RenderJSON(organizations); err != nil {
			panic(err)
		}
		return
	}

	t := clioutput.NewTable()
	t.AppendHeader(table.Row{"ID", "Name", "Created"})
	for _, org := range *organizations {
		t.AppendRow(table.Row{org.ID, org.Name, clioutput.TimeUTC(org.CreatedAt)})
	}
	clioutput.PrintTable(t)
	clioutput.PrintTotal("organization(s)", len(*organizations))
}
