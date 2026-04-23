package main

import (
	"context"
	"fmt"

	"github.com/openziti/agora/internal/api"
	"github.com/spf13/cobra"
)

func init() {
	workgroupMemberCmd.AddCommand(newWorkgroupMemberAddCommand().cmd)
}

type workgroupMemberAddCommand struct {
	role string
	cmd  *cobra.Command
}

func newWorkgroupMemberAddCommand() *workgroupMemberAddCommand {
	cmd := &cobra.Command{
		Use:   "add <name|wg_...> <account-email>",
		Short: "Add an account to a workgroup",
		Args:  cobra.ExactArgs(2),
	}
	command := &workgroupMemberAddCommand{cmd: cmd}
	cmd.Flags().StringVar(&command.role, "role", "member", "Membership role: member or admin")
	cmd.Run = command.run
	return command
}

func (cmd *workgroupMemberAddCommand) run(_ *cobra.Command, args []string) {
	root := requireEnabledRoot()
	client := openEnvironmentAPIClient(root)
	wgID := resolveWorkgroupID(client, args[0])
	email := args[1]

	req := &api.AddWorkgroupMemberRequest{AccountEmail: email}
	req.Role.SetTo(api.WorkgroupMembershipRole(cmd.role))

	res, err := client.AddWorkgroupMember(context.Background(), req, api.AddWorkgroupMemberParams{WorkgroupId: wgID})
	panicIfErr(err)
	switch typed := res.(type) {
	case *api.WorkgroupMembership:
		fmt.Printf("added member '%s' to workgroup '%s' with role '%s' (membership_id='%s')\n", email, wgID, typed.Role, typed.ID)
	case *api.AddWorkgroupMemberBadRequest:
		panic(typed.Message)
	case *api.AddWorkgroupMemberForbidden:
		panic(typed.Message)
	case *api.AddWorkgroupMemberNotFound:
		panic(typed.Message)
	case *api.AddWorkgroupMemberConflict:
		panic(typed.Message)
	case *api.AddWorkgroupMemberUnauthorized:
		panic(typed.Message)
	case *api.AddWorkgroupMemberInternalServerError:
		panic(typed.Message)
	default:
		panic(fmt.Sprintf("unexpected add workgroup member response: %T", res))
	}
}
