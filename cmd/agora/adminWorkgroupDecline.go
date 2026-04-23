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
	adminWorkgroupCmd.AddCommand(newAdminWorkgroupDeclineCommand().cmd)
}

type adminWorkgroupDeclineCommand struct {
	org string
	yes bool
	cmd *cobra.Command
}

func newAdminWorkgroupDeclineCommand() *adminWorkgroupDeclineCommand {
	cmd := &cobra.Command{
		Use:   "decline <wg_...>",
		Short: "Decline an inter-org workgroup invitation on behalf of an organization",
		Args:  cobra.ExactArgs(1),
	}
	command := &adminWorkgroupDeclineCommand{cmd: cmd}
	cmd.Flags().StringVar(&command.org, "org", "", "Invited organization id (org_...)")
	cmd.Flags().BoolVarP(&command.yes, "yes", "y", false, "Skip interactive confirmation")
	panicIfErr(cmd.MarkFlagRequired("org"))
	cmd.Run = command.run
	return command
}

func (cmd *adminWorkgroupDeclineCommand) run(_ *cobra.Command, args []string) {
	wgID := args[0]

	if !cmd.yes {
		fmt.Printf("decline invitation for workgroup '%s' on behalf of org '%s'? Type 'yes' to confirm: ", wgID, cmd.org)
		reader := bufio.NewReader(os.Stdin)
		answer, _ := reader.ReadString('\n')
		if strings.TrimSpace(answer) != "yes" {
			fmt.Println("aborted")
			return
		}
	}

	client := openAdminAPIClient()
	res, err := client.DeclineWorkgroupInvitation(
		context.Background(),
		&api.DeclineWorkgroupInvitationRequest{},
		api.DeclineWorkgroupInvitationParams{WorkgroupId: wgID, OrganizationId: cmd.org},
	)
	panicIfErr(err)
	switch typed := res.(type) {
	case *api.AcknowledgeWorkgroupInvitationResponse:
		fmt.Printf("declined invitation workgroup='%s' org='%s'; workgroup state is now '%s'\n", wgID, cmd.org, typed.Workgroup.State)
	case *api.DeclineWorkgroupInvitationBadRequest:
		panic(typed.Message)
	case *api.DeclineWorkgroupInvitationNotFound:
		panic(typed.Message)
	case *api.DeclineWorkgroupInvitationConflict:
		panic(typed.Message)
	case *api.DeclineWorkgroupInvitationUnauthorized:
		panic(typed.Message)
	case *api.DeclineWorkgroupInvitationInternalServerError:
		panic(typed.Message)
	default:
		panic(fmt.Sprintf("unexpected decline invitation response: %T", res))
	}
}
