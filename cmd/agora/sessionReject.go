package main

import (
	"context"
	"fmt"

	"github.com/openziti/agora/internal/api"
	"github.com/spf13/cobra"
)

func init() {
	sessionCmd.AddCommand(newSessionRejectCommand().cmd)
}

type sessionRejectCommand struct {
	reason string
	cmd    *cobra.Command
}

func newSessionRejectCommand() *sessionRejectCommand {
	cmd := &cobra.Command{
		Use:   "reject <ses_...>",
		Short: "Reject a proposed session (provider)",
		Args:  cobra.ExactArgs(1),
	}
	command := &sessionRejectCommand{cmd: cmd}
	cmd.Flags().StringVar(&command.reason, "reason", "", "Optional reason stored as close_detail")
	cmd.Run = command.run
	return command
}

func (cmd *sessionRejectCommand) run(_ *cobra.Command, args []string) {
	root := requireEnabledRoot()
	client := openEnvironmentAPIClient(root)

	body := api.OptCloseSessionRequest{}
	if cmd.reason != "" {
		body.SetTo(api.CloseSessionRequest{Detail: api.NewOptString(cmd.reason)})
	}
	res, err := client.RejectSession(context.Background(), body, api.RejectSessionParams{SessionId: args[0]})
	panicIfErr(err)
	switch typed := res.(type) {
	case *api.RejectSessionNoContent:
		fmt.Printf("rejected session '%s'\n", args[0])
	case *api.RejectSessionForbidden:
		panic(typed.Message)
	case *api.RejectSessionNotFound:
		panic(typed.Message)
	case *api.RejectSessionConflict:
		panic(typed.Message)
	case *api.RejectSessionUnauthorized:
		panic(typed.Message)
	case *api.RejectSessionInternalServerError:
		panic(typed.Message)
	default:
		panic(fmt.Sprintf("unexpected reject session response: %T", res))
	}
}
