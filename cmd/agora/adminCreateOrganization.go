package main

import (
	"context"
	"fmt"

	"github.com/openziti/agora/internal/api"
	"github.com/spf13/cobra"
)

func init() {
	adminCreateCmd.AddCommand(newAdminCreateOrganizationCommand().cmd)
}

type adminCreateOrganizationCommand struct {
	cmd *cobra.Command
}

func newAdminCreateOrganizationCommand() *adminCreateOrganizationCommand {
	cmd := &cobra.Command{
		Use:     "organization <name>",
		Aliases: []string{"org"},
		Short:   "Create an organization through the admin API",
		Args:    cobra.ExactArgs(1),
	}
	command := &adminCreateOrganizationCommand{cmd: cmd}
	cmd.Run = command.run
	return command
}

func (cmd *adminCreateOrganizationCommand) run(_ *cobra.Command, args []string) {
	client := openAdminAPIClient()

	res, err := client.CreateOrganization(context.Background(), &api.CreateOrganizationRequest{Name: args[0]})
	if err != nil {
		panic(err)
	}
	org, ok := res.(*api.Organization)
	if !ok {
		panic(fmt.Sprintf("unexpected create organization response: %T", res))
	}

	fmt.Printf("%s\t%s\n", org.ID, org.Name)
}
