package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/internal/clioutput"
	"github.com/spf13/cobra"
)

func init() {
	adminWorkgroupCmd.AddCommand(newAdminWorkgroupCreateCommand().cmd)
}

type adminWorkgroupCreateCommand struct {
	ownerOrg     string
	initialAdmin string
	scope        string
	description  string
	invites      []string
	jsonOutput   bool
	cmd          *cobra.Command
}

func newAdminWorkgroupCreateCommand() *adminWorkgroupCreateCommand {
	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create a workgroup",
		Args:  cobra.ExactArgs(1),
	}
	command := &adminWorkgroupCreateCommand{cmd: cmd}
	cmd.Flags().StringVar(&command.ownerOrg, "owner-org", "", "Owner organization id (org_...)")
	cmd.Flags().StringVar(&command.initialAdmin, "initial-admin", "", "Account id (ac_...) of the initial workgroup admin in the owner org")
	cmd.Flags().StringVar(&command.scope, "scope", "intra-org", "Workgroup scope: intra-org or inter-org")
	cmd.Flags().StringVar(&command.description, "description", "", "Optional human-readable description")
	cmd.Flags().StringSliceVar(&command.invites, "invite", nil, "Participating organization id (org_...); repeatable; required when --scope=inter-org")
	cmd.Flags().BoolVarP(&command.jsonOutput, "json", "j", false, "Output raw JSON")
	panicIfErr(cmd.MarkFlagRequired("owner-org"))
	panicIfErr(cmd.MarkFlagRequired("initial-admin"))
	cmd.Run = command.run
	return command
}

func (cmd *adminWorkgroupCreateCommand) run(_ *cobra.Command, args []string) {
	name := strings.TrimSpace(args[0])
	client := openAdminAPIClient()

	req := &api.CreateWorkgroupRequest{
		Name:                         name,
		Scope:                        api.CreateWorkgroupRequestScope(cmd.scope),
		OwnerOrganizationId:          cmd.ownerOrg,
		InitialAdminAccountId:        cmd.initialAdmin,
		ParticipatingOrganizationIds: cmd.invites,
	}
	if cmd.description != "" {
		req.Description.SetTo(cmd.description)
	}

	res, err := client.CreateWorkgroup(context.Background(), req)
	panicIfErr(err)

	created, ok := res.(*api.CreateWorkgroupResponse)
	if !ok {
		switch typed := res.(type) {
		case *api.CreateWorkgroupBadRequest:
			panic(typed.Message)
		case *api.CreateWorkgroupConflict:
			panic(typed.Message)
		case *api.CreateWorkgroupUnauthorized:
			panic(typed.Message)
		case *api.CreateWorkgroupInternalServerError:
			panic(typed.Message)
		}
		panic(fmt.Sprintf("unexpected create workgroup response: %T", res))
	}

	if cmd.jsonOutput {
		panicIfErr(clioutput.RenderJSON(created))
		return
	}
	fmt.Printf("created workgroup '%s' (id=%s scope=%s state=%s owner_org=%s)\n", created.Workgroup.Name, created.Workgroup.ID, created.Workgroup.Scope, created.Workgroup.State, created.Workgroup.OwnerOrganizationId)
	for _, inv := range created.Invitations {
		fmt.Printf("  invitation: org=%s state=%s id=%s\n", inv.OrganizationId, inv.State, inv.ID)
	}
}
