package main

import (
	"context"
	"fmt"

	"github.com/openziti/agora/internal/api"
	"github.com/spf13/cobra"
)

func init() {
	sessionCmd.AddCommand(newSessionCloseCommand().cmd)
}

type sessionCloseCommand struct {
	reason string
	cmd    *cobra.Command
}

func newSessionCloseCommand() *sessionCloseCommand {
	cmd := &cobra.Command{
		Use:   "close <ses_...>",
		Short: "Close a session (either participant)",
		Args:  cobra.ExactArgs(1),
	}
	command := &sessionCloseCommand{cmd: cmd}
	cmd.Flags().StringVar(&command.reason, "reason", "", "Optional reason stored as close_detail")
	cmd.Run = command.run
	return command
}

func (cmd *sessionCloseCommand) run(_ *cobra.Command, args []string) {
	root := requireEnabledRoot()
	client := openEnvironmentAPIClient(root)

	body := api.OptCloseSessionRequest{}
	if cmd.reason != "" {
		body.SetTo(api.CloseSessionRequest{Detail: api.NewOptString(cmd.reason)})
	}
	res, err := client.CloseSession(context.Background(), body, api.CloseSessionParams{SessionId: args[0]})
	panicIfErr(err)
	switch typed := res.(type) {
	case *api.CloseSessionNoContent:
		fmt.Printf("closed session '%s'\n", args[0])
	case *api.CloseSessionBadRequest:
		panic(typed.Message)
	case *api.CloseSessionForbidden:
		panic(typed.Message)
	case *api.CloseSessionNotFound:
		panic(typed.Message)
	case *api.CloseSessionUnauthorized:
		panic(typed.Message)
	case *api.CloseSessionInternalServerError:
		panic(typed.Message)
	default:
		panic(fmt.Sprintf("unexpected close session response: %T", res))
	}
}
