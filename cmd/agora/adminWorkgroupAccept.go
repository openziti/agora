package main

import (
	"context"
	"fmt"

	"github.com/openziti/agora/internal/api"
	"github.com/spf13/cobra"
)

func init() {
	adminWorkgroupCmd.AddCommand(newAdminWorkgroupAcceptCommand().cmd)
}

type adminWorkgroupAcceptCommand struct {
	org          string
	initialAdmin string
	cmd          *cobra.Command
}

func newAdminWorkgroupAcceptCommand() *adminWorkgroupAcceptCommand {
	cmd := &cobra.Command{
		Use:   "accept <wg_...>",
		Short: "Accept an inter-org workgroup invitation on behalf of an organization",
		Args:  cobra.ExactArgs(1),
	}
	command := &adminWorkgroupAcceptCommand{cmd: cmd}
	cmd.Flags().StringVar(&command.org, "org", "", "Invited organization id (org_...)")
	cmd.Flags().StringVar(&command.initialAdmin, "initial-admin", "", "Account id (ac_...) of the initial workgroup admin in the invited org")
	panicIfErr(cmd.MarkFlagRequired("org"))
	panicIfErr(cmd.MarkFlagRequired("initial-admin"))
	cmd.Run = command.run
	return command
}

func (cmd *adminWorkgroupAcceptCommand) run(_ *cobra.Command, args []string) {
	wgID := args[0]
	client := openAdminAPIClient()

	res, err := client.AcceptWorkgroupInvitation(
		context.Background(),
		&api.AcceptWorkgroupInvitationRequest{InitialAdminAccountId: cmd.initialAdmin},
		api.AcceptWorkgroupInvitationParams{WorkgroupId: wgID, OrganizationId: cmd.org},
	)
	panicIfErr(err)
	switch typed := res.(type) {
	case *api.AcknowledgeWorkgroupInvitationResponse:
		fmt.Printf("accepted invitation workgroup='%s' org='%s'; workgroup state is now '%s'\n", wgID, cmd.org, typed.Workgroup.State)
	case *api.AcceptWorkgroupInvitationBadRequest:
		panic(typed.Message)
	case *api.AcceptWorkgroupInvitationNotFound:
		panic(typed.Message)
	case *api.AcceptWorkgroupInvitationConflict:
		panic(typed.Message)
	case *api.AcceptWorkgroupInvitationUnauthorized:
		panic(typed.Message)
	case *api.AcceptWorkgroupInvitationInternalServerError:
		panic(typed.Message)
	default:
		panic(fmt.Sprintf("unexpected accept invitation response: %T", res))
	}
}
