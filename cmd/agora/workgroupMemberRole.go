package main

import (
	"context"
	"fmt"

	"github.com/openziti/agora/internal/api"
	"github.com/spf13/cobra"
)

func init() {
	workgroupMemberCmd.AddCommand(newWorkgroupMemberRoleCommand().cmd)
}

type workgroupMemberRoleCommand struct {
	cmd *cobra.Command
}

func newWorkgroupMemberRoleCommand() *workgroupMemberRoleCommand {
	cmd := &cobra.Command{
		Use:   "role <name|wg_...> <account-email> <member|admin>",
		Short: "Change a member's role in a workgroup",
		Args:  cobra.ExactArgs(3),
	}
	command := &workgroupMemberRoleCommand{cmd: cmd}
	cmd.Run = command.run
	return command
}

func (cmd *workgroupMemberRoleCommand) run(_ *cobra.Command, args []string) {
	root := requireEnabledRoot()
	client := openEnvironmentAPIClient(root)
	wgID := resolveWorkgroupID(client, args[0])
	email := args[1]
	role := args[2]

	membershipID := resolveWorkgroupMembershipID(client, wgID, email)

	res, err := client.ChangeWorkgroupMembershipRole(
		context.Background(),
		&api.ChangeWorkgroupMembershipRoleRequest{Role: api.WorkgroupMembershipRole(role)},
		api.ChangeWorkgroupMembershipRoleParams{WorkgroupId: wgID, MembershipId: membershipID},
	)
	panicIfErr(err)
	switch typed := res.(type) {
	case *api.WorkgroupMembership:
		fmt.Printf("set role for '%s' in workgroup '%s' to '%s'\n", email, wgID, typed.Role)
	case *api.ChangeWorkgroupMembershipRoleBadRequest:
		panic(typed.Message)
	case *api.ChangeWorkgroupMembershipRoleForbidden:
		panic(typed.Message)
	case *api.ChangeWorkgroupMembershipRoleNotFound:
		panic(typed.Message)
	case *api.ChangeWorkgroupMembershipRoleConflict:
		panic(typed.Message)
	case *api.ChangeWorkgroupMembershipRoleUnauthorized:
		panic(typed.Message)
	case *api.ChangeWorkgroupMembershipRoleInternalServerError:
		panic(typed.Message)
	default:
		panic(fmt.Sprintf("unexpected change workgroup membership role response: %T", res))
	}
}
