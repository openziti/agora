package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/internal/clioutput"
	"github.com/spf13/cobra"
)

func init() {
	workgroupCmd.AddCommand(newWorkgroupListCommand().cmd)
}

type workgroupListCommand struct {
	jsonOutput bool
	cmd        *cobra.Command
}

func newWorkgroupListCommand() *workgroupListCommand {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List workgroups visible to the current account",
		Args:  cobra.NoArgs,
	}
	command := &workgroupListCommand{cmd: cmd}
	cmd.Flags().BoolVarP(&command.jsonOutput, "json", "j", false, "Output raw JSON instead of a table")
	cmd.Run = command.run
	return command
}

func (cmd *workgroupListCommand) run(_ *cobra.Command, _ []string) {
	root := requireEnabledRoot()
	client := openEnvironmentAPIClient(root)

	res, err := client.ListWorkgroups(context.Background())
	panicIfErr(err)

	listing, ok := res.(*api.ListWorkgroupsResponse)
	if !ok {
		switch typed := res.(type) {
		case *api.ListWorkgroupsUnauthorized:
			panic(typed.Message)
		case *api.ListWorkgroupsInternalServerError:
			panic(typed.Message)
		}
		panic(fmt.Sprintf("unexpected list workgroups response: %T", res))
	}

	if cmd.jsonOutput {
		panicIfErr(clioutput.RenderJSON(listing))
		return
	}

	t := clioutput.NewTable()
	t.AppendHeader(table.Row{"Name", "ID", "Scope", "State", "Owner Org", "Participating Orgs", "Updated"})
	for _, wg := range *listing {
		t.AppendRow(table.Row{
			wg.Name,
			wg.ID,
			wg.Scope,
			wg.State,
			wg.OwnerOrganizationId,
			strings.Join(wg.ParticipatingOrganizationIds, ","),
			clioutput.TimeUTC(wg.UpdatedAt),
		})
	}
	clioutput.PrintTable(t)
	clioutput.PrintTotal("workgroup(s)", len(*listing))
}
