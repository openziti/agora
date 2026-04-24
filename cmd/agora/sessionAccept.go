package main

import (
	"context"
	"fmt"

	"github.com/openziti/agora/internal/api"
	"github.com/spf13/cobra"
)

func init() {
	sessionCmd.AddCommand(newSessionAcceptCommand().cmd)
}

type sessionAcceptCommand struct {
	environmentID string
	cmd           *cobra.Command
}

func newSessionAcceptCommand() *sessionAcceptCommand {
	cmd := &cobra.Command{
		Use:   "accept <ses_...>",
		Short: "Accept a proposed session (provider)",
		Args:  cobra.ExactArgs(1),
	}
	command := &sessionAcceptCommand{cmd: cmd}
	cmd.Flags().StringVar(&command.environmentID, "environment", "", "Environment id that hosts the backing tunnel (defaults to the current environment)")
	cmd.Run = command.run
	return command
}

func (cmd *sessionAcceptCommand) run(_ *cobra.Command, args []string) {
	root := requireEnabledRoot()
	client := openEnvironmentAPIClient(root)

	envID := cmd.environmentID
	if envID == "" {
		envID = root.Environment().EnvironmentID
	}
	res, err := client.AcceptSession(context.Background(),
		&api.AcceptSessionRequest{EnvironmentId: envID},
		api.AcceptSessionParams{SessionId: args[0]})
	panicIfErr(err)
	switch typed := res.(type) {
	case *api.Session:
		tunnel := ""
		if typed.TunnelId.Set {
			tunnel = typed.TunnelId.Value
		}
		fmt.Printf("accepted session '%s' (state=%s tunnel=%s)\n", typed.ID, typed.State, tunnel)
	case *api.AcceptSessionBadRequest:
		panic(typed.Message)
	case *api.AcceptSessionForbidden:
		panic(typed.Message)
	case *api.AcceptSessionNotFound:
		panic(typed.Message)
	case *api.AcceptSessionConflict:
		panic(typed.Message)
	case *api.AcceptSessionUnauthorized:
		panic(typed.Message)
	case *api.AcceptSessionInternalServerError:
		panic(typed.Message)
	default:
		panic(fmt.Sprintf("unexpected accept session response: %T", res))
	}
}
