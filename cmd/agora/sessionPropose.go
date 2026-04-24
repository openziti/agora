package main

import (
	"context"
	"fmt"

	"github.com/openziti/agora/internal/api"
	"github.com/spf13/cobra"
)

func init() {
	sessionCmd.AddCommand(newSessionProposeCommand().cmd)
}

type sessionProposeCommand struct {
	workgroup string
	message   string
	cmd       *cobra.Command
}

func newSessionProposeCommand() *sessionProposeCommand {
	cmd := &cobra.Command{
		Use:   "propose <adv_...>",
		Short: "Propose a session against an advertisement",
		Args:  cobra.ExactArgs(1),
	}
	command := &sessionProposeCommand{cmd: cmd}
	cmd.Flags().StringVar(&command.workgroup, "workgroup", "", "Workgroup name or wg_... id (required)")
	cmd.Flags().StringVar(&command.message, "message", "", "Optional proposer message")
	panicIfErr(cmd.MarkFlagRequired("workgroup"))
	cmd.Run = command.run
	return command
}

func (cmd *sessionProposeCommand) run(_ *cobra.Command, args []string) {
	root := requireEnabledRoot()
	client := openEnvironmentAPIClient(root)
	wgID := resolveWorkgroupID(client, cmd.workgroup)

	req := &api.ProposeSessionRequest{
		AdvertisementId: args[0],
		WorkgroupId:     wgID,
	}
	if cmd.message != "" {
		req.ProposerMessage.SetTo(cmd.message)
	}
	res, err := client.ProposeSession(context.Background(), req)
	panicIfErr(err)
	switch typed := res.(type) {
	case *api.Session:
		fmt.Printf("proposed session '%s' (state=%s, workgroup=%s)\n", typed.ID, typed.State, typed.WorkgroupId)
	case *api.ProposeSessionBadRequest:
		panic(typed.Message)
	case *api.ProposeSessionNotFound:
		panic(typed.Message)
	case *api.ProposeSessionConflict:
		panic(typed.Message)
	case *api.ProposeSessionUnauthorized:
		panic(typed.Message)
	case *api.ProposeSessionInternalServerError:
		panic(typed.Message)
	default:
		panic(fmt.Sprintf("unexpected propose session response: %T", res))
	}
}
