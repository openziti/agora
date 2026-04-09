package main

import (
	"context"
	"fmt"

	"github.com/openziti/agora/internal/api"
	"github.com/spf13/cobra"
)

func init() {
	adminCreateCmd.AddCommand(newAdminCreateUserCommand().cmd)
}

type adminCreateUserCommand struct {
	cmd         *cobra.Command
	displayName string
	role        string
	status      string
}

func newAdminCreateUserCommand() *adminCreateUserCommand {
	cmd := &cobra.Command{
		Use:     "user <organizationId> <email> <password>",
		Aliases: []string{"account"},
		Short:   "Create a user through the admin API",
		Args:    cobra.ExactArgs(3),
	}
	command := &adminCreateUserCommand{cmd: cmd}
	cmd.Flags().StringVar(&command.displayName, "display-name", "", "Optional display name")
	cmd.Flags().StringVar(&command.role, "role", "", "Optional role (admin|member)")
	cmd.Flags().StringVar(&command.status, "status", "", "Optional status (active|disabled)")
	cmd.Run = command.run
	return command
}

func (cmd *adminCreateUserCommand) run(_ *cobra.Command, args []string) {
	client := openAdminAPIClient()

	orgID := args[0]

	req := &api.CreateAccountRequest{
		Email:    args[1],
		Password: args[2],
	}
	if cmd.displayName != "" {
		req.DisplayName.SetTo(cmd.displayName)
	}
	if cmd.role != "" {
		req.Role.SetTo(api.CreateAccountRequestRole(cmd.role))
	}
	if cmd.status != "" {
		req.Status.SetTo(api.CreateAccountRequestStatus(cmd.status))
	}

	res, err := client.CreateAccount(context.Background(), req, api.CreateAccountParams{OrganizationId: orgID})
	if err != nil {
		panic(err)
	}
	tokenRes, ok := res.(*api.AccountTokenResponse)
	if !ok {
		panic(fmt.Sprintf("unexpected create user response: %T", res))
	}

	listRes, err := client.ListAccounts(context.Background(), api.ListAccountsParams{OrganizationId: orgID})
	if err != nil {
		panic(err)
	}
	accounts, ok := listRes.(*api.ListAccountsResponse)
	if !ok {
		panic(fmt.Sprintf("unexpected list accounts response: %T", listRes))
	}

	for _, account := range *accounts {
		if account.Email == args[1] {
			fmt.Printf("%s\t%s\t%s\n", account.ID, account.Email, tokenRes.AccountToken)
			return
		}
	}

	panic("created user was not returned by list accounts")
}
