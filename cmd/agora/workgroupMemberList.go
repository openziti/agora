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
	workgroupMemberCmd.AddCommand(newWorkgroupMemberListCommand().cmd)
}

type workgroupMemberListCommand struct {
	jsonOutput bool
	cmd        *cobra.Command
}

func newWorkgroupMemberListCommand() *workgroupMemberListCommand {
	cmd := &cobra.Command{
		Use:   "list <name|wg_...>",
		Short: "List members of a workgroup",
		Args:  cobra.ExactArgs(1),
	}
	command := &workgroupMemberListCommand{cmd: cmd}
	cmd.Flags().BoolVarP(&command.jsonOutput, "json", "j", false, "Output raw JSON instead of a table")
	cmd.Run = command.run
	return command
}

func (cmd *workgroupMemberListCommand) run(_ *cobra.Command, args []string) {
	root := requireEnabledRoot()
	client := openEnvironmentAPIClient(root)
	wgID := resolveWorkgroupID(client, args[0])

	res, err := client.ListWorkgroupMembers(context.Background(), api.ListWorkgroupMembersParams{WorkgroupId: wgID})
	panicIfErr(err)

	listing, ok := res.(*api.ListWorkgroupMembersResponse)
	if !ok {
		switch typed := res.(type) {
		case *api.ListWorkgroupMembersNotFound:
			panic(typed.Message)
		case *api.ListWorkgroupMembersUnauthorized:
			panic(typed.Message)
		case *api.ListWorkgroupMembersInternalServerError:
			panic(typed.Message)
		}
		panic(fmt.Sprintf("unexpected list workgroup members response: %T", res))
	}

	if cmd.jsonOutput {
		panicIfErr(clioutput.RenderJSON(listing))
		return
	}

	t := clioutput.NewTable()
	t.AppendHeader(table.Row{"Email", "Account ID", "Organization", "Role", "Joined"})
	for _, m := range *listing {
		email := ""
		if m.Email.Set {
			email = m.Email.Value
		}
		t.AppendRow(table.Row{
			email,
			m.AccountId,
			m.OrganizationId,
			m.Role,
			clioutput.TimeUTC(m.JoinedAt),
		})
	}
	clioutput.PrintTable(t)
	clioutput.PrintTotal("member(s)", len(*listing))
}
