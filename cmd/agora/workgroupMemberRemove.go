package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/openziti/agora/internal/api"
	"github.com/spf13/cobra"
)

func init() {
	workgroupMemberCmd.AddCommand(newWorkgroupMemberRemoveCommand().cmd)
}

type workgroupMemberRemoveCommand struct {
	yes bool
	cmd *cobra.Command
}

func newWorkgroupMemberRemoveCommand() *workgroupMemberRemoveCommand {
	cmd := &cobra.Command{
		Use:   "remove <name|wg_...> <account-email>",
		Short: "Remove an account from a workgroup",
		Args:  cobra.ExactArgs(2),
	}
	command := &workgroupMemberRemoveCommand{cmd: cmd}
	cmd.Flags().BoolVarP(&command.yes, "yes", "y", false, "Skip interactive confirmation")
	cmd.Run = command.run
	return command
}

func (cmd *workgroupMemberRemoveCommand) run(_ *cobra.Command, args []string) {
	root := requireEnabledRoot()
	client := openEnvironmentAPIClient(root)
	wgID := resolveWorkgroupID(client, args[0])
	email := args[1]

	membershipID := resolveWorkgroupMembershipID(client, wgID, email)

	if !cmd.yes {
		fmt.Printf("remove member '%s' from workgroup '%s'? Type 'yes' to confirm: ", email, wgID)
		reader := bufio.NewReader(os.Stdin)
		answer, _ := reader.ReadString('\n')
		if strings.TrimSpace(answer) != "yes" {
			fmt.Println("aborted")
			return
		}
	}

	res, err := client.RemoveWorkgroupMember(context.Background(), api.RemoveWorkgroupMemberParams{WorkgroupId: wgID, MembershipId: membershipID})
	panicIfErr(err)
	switch typed := res.(type) {
	case *api.RemoveWorkgroupMemberNoContent:
		fmt.Printf("removed member '%s' from workgroup '%s'\n", email, wgID)
	case *api.RemoveWorkgroupMemberForbidden:
		panic(typed.Message)
	case *api.RemoveWorkgroupMemberNotFound:
		panic(typed.Message)
	case *api.RemoveWorkgroupMemberConflict:
		panic(typed.Message)
	case *api.RemoveWorkgroupMemberUnauthorized:
		panic(typed.Message)
	case *api.RemoveWorkgroupMemberInternalServerError:
		panic(typed.Message)
	default:
		panic(fmt.Sprintf("unexpected remove workgroup member response: %T", res))
	}
}

func resolveWorkgroupMembershipID(client *api.Client, wgID, email string) string {
	res, err := client.ListWorkgroupMembers(context.Background(), api.ListWorkgroupMembersParams{WorkgroupId: wgID})
	panicIfErr(err)
	listing, ok := res.(*api.ListWorkgroupMembersResponse)
	if !ok {
		panic(fmt.Sprintf("unexpected list workgroup members response: %T", res))
	}
	for _, m := range *listing {
		if m.Email.Set && strings.EqualFold(m.Email.Value, email) {
			return m.ID
		}
	}
	panic(fmt.Sprintf("no membership found for account '%s' in workgroup '%s' (cross-org accounts cannot be resolved by email)", email, wgID))
}
