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
	adminWorkgroupCmd.AddCommand(newAdminWorkgroupListCommand().cmd)
}

type adminWorkgroupListCommand struct {
	state      string
	ownerOrg   string
	invitedOrg string
	jsonOutput bool
	cmd        *cobra.Command
}

func newAdminWorkgroupListCommand() *adminWorkgroupListCommand {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List workgroups across all organizations (admin)",
		Args:  cobra.NoArgs,
	}
	command := &adminWorkgroupListCommand{cmd: cmd}
	cmd.Flags().StringVar(&command.state, "state", "", "Filter by state: pending, active, declined")
	cmd.Flags().StringVar(&command.ownerOrg, "owner-org", "", "Filter by owner organization id")
	cmd.Flags().StringVar(&command.invitedOrg, "invited-org", "", "Filter by invited organization id")
	cmd.Flags().BoolVarP(&command.jsonOutput, "json", "j", false, "Output raw JSON")
	cmd.Run = command.run
	return command
}

func (cmd *adminWorkgroupListCommand) run(_ *cobra.Command, _ []string) {
	client := openAdminAPIClient()

	params := api.ListAdminWorkgroupsParams{}
	if cmd.state != "" {
		params.State.SetTo(api.ListAdminWorkgroupsState(cmd.state))
	}
	if cmd.ownerOrg != "" {
		params.OwnerOrganizationId.SetTo(cmd.ownerOrg)
	}
	if cmd.invitedOrg != "" {
		params.InvitedOrganizationId.SetTo(cmd.invitedOrg)
	}

	res, err := client.ListAdminWorkgroups(context.Background(), params)
	panicIfErr(err)

	listing, ok := res.(*api.ListAdminWorkgroupsResponse)
	if !ok {
		switch typed := res.(type) {
		case *api.ListAdminWorkgroupsBadRequest:
			panic(typed.Message)
		case *api.ListAdminWorkgroupsUnauthorized:
			panic(typed.Message)
		case *api.ListAdminWorkgroupsInternalServerError:
			panic(typed.Message)
		}
		panic(fmt.Sprintf("unexpected admin list workgroups response: %T", res))
	}

	if cmd.jsonOutput {
		panicIfErr(clioutput.RenderJSON(listing))
		return
	}

	t := clioutput.NewTable()
	t.AppendHeader(table.Row{"Name", "ID", "Scope", "State", "Owner Org", "Invitations", "Updated"})
	for _, entry := range *listing {
		invSummary := summarizeInvitations(entry.Invitations)
		t.AppendRow(table.Row{
			entry.Workgroup.Name,
			entry.Workgroup.ID,
			entry.Workgroup.Scope,
			entry.Workgroup.State,
			entry.Workgroup.OwnerOrganizationId,
			invSummary,
			clioutput.TimeUTC(entry.Workgroup.UpdatedAt),
		})
	}
	clioutput.PrintTable(t)
	clioutput.PrintTotal("workgroup(s)", len(*listing))
}

func summarizeInvitations(invitations []api.WorkgroupInvitation) string {
	if len(invitations) == 0 {
		return "-"
	}
	var pending, accepted, declined int
	for _, inv := range invitations {
		switch inv.State {
		case api.WorkgroupInvitationStatePending:
			pending++
		case api.WorkgroupInvitationStateAccepted:
			accepted++
		case api.WorkgroupInvitationStateDeclined:
			declined++
		}
	}
	return fmt.Sprintf("p=%d a=%d d=%d", pending, accepted, declined)
}
