package main

import (
	"context"

	"github.com/openziti/agora/internal/api"
	"github.com/spf13/cobra"
)

func init() {
	adminDeleteCmd.AddCommand(newAdminDeleteOrganizationCommand().cmd)
}

type adminDeleteOrganizationCommand struct {
	cmd *cobra.Command
}

func newAdminDeleteOrganizationCommand() *adminDeleteOrganizationCommand {
	cmd := &cobra.Command{
		Use:     "organization <organizationId>",
		Aliases: []string{"org"},
		Short:   "Delete an organization through the admin API",
		Args:    cobra.ExactArgs(1),
	}
	command := &adminDeleteOrganizationCommand{cmd: cmd}
	cmd.Run = command.run
	return command
}

func (cmd *adminDeleteOrganizationCommand) run(_ *cobra.Command, args []string) {
	client := openAdminAPIClient()

	if _, err := client.DeleteOrganization(context.Background(), api.DeleteOrganizationParams{OrganizationId: args[0]}); err != nil {
		panic(err)
	}
}
