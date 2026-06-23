package main

import (
	"context"
	"fmt"

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
	cmd.RunE = command.runE
	return command
}

func (cmd *adminDeleteUserCommand) runE(_ *cobra.Command, args []string) error {
	client := openAdminAPIClient()

	res, err := client.DeleteAccount(context.Background(), api.DeleteAccountParams{
		OrganizationId: args[0],
		AccountId:      args[1],
	})
	if err != nil {
		return err
	}
	switch typed := res.(type) {
	case *api.DeleteAccountNoContent:
		fmt.Printf("deleted account '%s'\n", args[1])
		return nil
	case *api.DeleteAccountConflict:
		return fmt.Errorf("%s", typed.Message)
	case *api.DeleteAccountNotFound:
		return fmt.Errorf("%s", typed.Message)
	case *api.DeleteAccountUnauthorized:
		return fmt.Errorf("%s", typed.Message)
	case *api.DeleteAccountInternalServerError:
		return fmt.Errorf("%s", typed.Message)
	default:
		return fmt.Errorf("unexpected delete account response: %T", res)
	}
}
