package main

import (
	"context"
	"fmt"

	"github.com/openziti/agora/internal/api"
	"github.com/spf13/cobra"
)

func init() {
	adminSessionCmd.AddCommand(newAdminSessionCloseCommand().cmd)
}

type adminSessionCloseCommand struct {
	reason string
	cmd    *cobra.Command
}

func newAdminSessionCloseCommand() *adminSessionCloseCommand {
	cmd := &cobra.Command{
		Use:   "close <ses_...>",
		Short: "Administrator-initiated session close",
		Args:  cobra.ExactArgs(1),
	}
	command := &adminSessionCloseCommand{cmd: cmd}
	cmd.Flags().StringVar(&command.reason, "reason", "", "Optional reason stored as close_detail")
	cmd.Run = command.run
	return command
}

func (cmd *adminSessionCloseCommand) run(_ *cobra.Command, args []string) {
	client := openAdminAPIClient()
	body := api.OptCloseSessionRequest{}
	if cmd.reason != "" {
		body.SetTo(api.CloseSessionRequest{Detail: api.NewOptString(cmd.reason)})
	}
	res, err := client.AdminCloseSession(context.Background(), body, api.AdminCloseSessionParams{SessionId: args[0]})
	panicIfErr(err)
	switch typed := res.(type) {
	case *api.AdminCloseSessionNoContent:
		fmt.Printf("admin-closed session '%s'\n", args[0])
	case *api.AdminCloseSessionNotFound:
		panic(typed.Message)
	case *api.AdminCloseSessionUnauthorized:
		panic(typed.Message)
	case *api.AdminCloseSessionInternalServerError:
		panic(typed.Message)
	default:
		panic(fmt.Sprintf("unexpected admin close session response: %T", res))
	}
}
