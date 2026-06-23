package main

import (
	"context"
	"fmt"

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
	cmd.RunE = command.runE
	return command
}

func (cmd *adminDeleteOrganizationCommand) runE(_ *cobra.Command, args []string) error {
	client := openAdminAPIClient()

	res, err := client.DeleteOrganization(context.Background(), api.DeleteOrganizationParams{OrganizationId: args[0]})
	if err != nil {
		return err
	}
	switch typed := res.(type) {
	case *api.DeleteOrganizationNoContent:
		fmt.Printf("deleted organization '%s'\n", args[0])
		return nil
	case *api.DeleteOrganizationConflict:
		return fmt.Errorf("%s", typed.Message)
	case *api.DeleteOrganizationNotFound:
		return fmt.Errorf("%s", typed.Message)
	case *api.DeleteOrganizationUnauthorized:
		return fmt.Errorf("%s", typed.Message)
	case *api.DeleteOrganizationInternalServerError:
		return fmt.Errorf("%s", typed.Message)
	default:
		return fmt.Errorf("unexpected delete organization response: %T", res)
	}
}
