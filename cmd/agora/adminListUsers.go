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
	adminListCmd.AddCommand(newAdminListUsersCommand().cmd)
}

type adminListUsersCommand struct {
	cmd            *cobra.Command
	jsonOutput     bool
	showIDs        bool
	organizationID string
}

func newAdminListUsersCommand() *adminListUsersCommand {
	cmd := &cobra.Command{
		Use:     "users",
		Aliases: []string{"accounts"},
		Short:   "List users",
		Args:    cobra.NoArgs,
	}
	command := &adminListUsersCommand{cmd: cmd}
	cmd.Flags().BoolVar(&command.jsonOutput, "json", false, "Output raw JSON instead of a table")
	cmd.Flags().BoolVar(&command.showIDs, "show-ids", false, "Include raw user and organization identifiers in table output")
	cmd.Flags().StringVar(&command.organizationID, "organization-id", "", "Optional organization ID filter")
	cmd.Run = command.run
	return command
}

func (cmd *adminListUsersCommand) run(_ *cobra.Command, _ []string) {
	client := openAdminAPIClient()

	params := api.ListUsersParams{}
	if cmd.organizationID != "" {
		params.OrganizationId.SetTo(cmd.organizationID)
	}

	res, err := client.ListUsers(context.Background(), params)
	if err != nil {
		panic(err)
	}
	users, ok := res.(*api.ListAccountsResponse)
	if !ok {
		panic(fmt.Sprintf("unexpected list users response: %T", res))
	}

	if cmd.jsonOutput {
		if err := clioutput.RenderJSON(users); err != nil {
			panic(err)
		}
		return
	}

	orgNames := cmd.organizationNamesByID(client)

	t := clioutput.NewTable()
	if cmd.showIDs {
		t.AppendHeader(table.Row{"Email", "Organization", "Display Name", "Role", "Status", "Created", "User ID", "Organization ID"})
	} else {
		t.AppendHeader(table.Row{"Email", "Organization", "Display Name", "Role", "Status", "Created"})
	}
	for _, user := range *users {
		displayName, _ := user.DisplayName.Get()
		orgName := clioutput.StringOrDash(orgNames[user.OrganizationId])
		if cmd.showIDs {
			t.AppendRow(table.Row{
				user.Email,
				orgName,
				clioutput.StringOrDash(displayName),
				user.Role,
				user.Status,
				clioutput.TimeUTC(user.CreatedAt),
				user.ID,
				user.OrganizationId,
			})
		} else {
			t.AppendRow(table.Row{
				user.Email,
				orgName,
				clioutput.StringOrDash(displayName),
				user.Role,
				user.Status,
				clioutput.TimeUTC(user.CreatedAt),
			})
		}
	}
	clioutput.PrintTable(t)
	clioutput.PrintTotal("user(s)", len(*users))
}

func (cmd *adminListUsersCommand) organizationNamesByID(client *api.Client) map[string]string {
	result := map[string]string{}

	res, err := client.ListOrganizations(context.Background())
	if err != nil {
		panic(err)
	}
	organizations, ok := res.(*api.ListOrganizationsResponse)
	if !ok {
		panic(fmt.Sprintf("unexpected list organizations response: %T", res))
	}
	for _, org := range *organizations {
		result[org.ID] = org.Name
	}

	return result
}
