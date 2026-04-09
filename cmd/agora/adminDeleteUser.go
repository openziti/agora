package main

import (
	"context"

	"github.com/openziti/agora/internal/api"
	"github.com/spf13/cobra"
)

func init() {
	adminDeleteCmd.AddCommand(newAdminDeleteUserCommand().cmd)
}

type adminDeleteUserCommand struct {
	cmd *cobra.Command
}

func newAdminDeleteUserCommand() *adminDeleteUserCommand {
	cmd := &cobra.Command{
		Use:     "user <organizationId> <accountId>",
		Aliases: []string{"account"},
		Short:   "Delete a user through the admin API",
		Args:    cobra.ExactArgs(2),
	}
	command := &adminDeleteUserCommand{cmd: cmd}
	cmd.Run = command.run
	return command
}

func (cmd *adminDeleteUserCommand) run(_ *cobra.Command, args []string) {
	client := openAdminAPIClient()

	if _, err := client.DeleteAccount(context.Background(), api.DeleteAccountParams{
		OrganizationId: args[0],
		AccountId:      args[1],
	}); err != nil {
		panic(err)
	}
}
