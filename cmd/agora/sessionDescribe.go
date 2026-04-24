package main

import (
	"context"
	"fmt"

	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/internal/clioutput"
	"github.com/spf13/cobra"
)

func init() {
	sessionCmd.AddCommand(newSessionDescribeCommand().cmd)
}

type sessionDescribeCommand struct {
	jsonOutput bool
	cmd        *cobra.Command
}

func newSessionDescribeCommand() *sessionDescribeCommand {
	cmd := &cobra.Command{
		Use:   "describe <ses_...>",
		Short: "Describe a session",
		Args:  cobra.ExactArgs(1),
	}
	command := &sessionDescribeCommand{cmd: cmd}
	cmd.Flags().BoolVarP(&command.jsonOutput, "json", "j", false, "Output raw JSON")
	cmd.Run = command.run
	return command
}

func (cmd *sessionDescribeCommand) run(_ *cobra.Command, args []string) {
	root := requireEnabledRoot()
	client := openEnvironmentAPIClient(root)
	res, err := client.GetSession(context.Background(), api.GetSessionParams{SessionId: args[0]})
	panicIfErr(err)
	sess, ok := res.(*api.Session)
	if !ok {
		switch typed := res.(type) {
		case *api.GetSessionNotFound:
			panic(typed.Message)
		case *api.GetSessionUnauthorized:
			panic(typed.Message)
		case *api.GetSessionInternalServerError:
			panic(typed.Message)
		}
		panic(fmt.Sprintf("unexpected get session response: %T", res))
	}
	if cmd.jsonOutput {
		panicIfErr(clioutput.RenderJSON(sess))
		return
	}

	fmt.Printf("session %s\n", sess.ID)
	fmt.Printf("  state         : %s\n", sess.State)
	fmt.Printf("  advertisement : %s\n", sess.AdvertisementId)
	fmt.Printf("  workgroup     : %s\n", sess.WorkgroupId)
	fmt.Printf("  provider      : %s (org %s)\n", sess.ProviderAccountId, sess.ProviderOrganizationId)
	fmt.Printf("  consumer      : %s (org %s)\n", sess.ConsumerAccountId, sess.ConsumerOrganizationId)
	fmt.Printf("  tunnel mode   : %s\n", sess.TunnelMode)
	if sess.TunnelId.Set {
		fmt.Printf("  tunnel id     : %s\n", sess.TunnelId.Value)
	}
	if sess.ProposerMessage.Set {
		fmt.Printf("  msg           : %s\n", sess.ProposerMessage.Value)
	}
	fmt.Printf("  proposed at   : %s\n", clioutput.TimeUTC(sess.ProposedAt))
	if sess.AcceptedAt.Set {
		fmt.Printf("  accepted at   : %s\n", clioutput.TimeUTC(sess.AcceptedAt.Value))
	}
	if sess.ClosedAt.Set {
		fmt.Printf("  closed at     : %s\n", clioutput.TimeUTC(sess.ClosedAt.Value))
	}
	if sess.CloseReason.Set {
		fmt.Printf("  close reason  : %s\n", sess.CloseReason.Value)
	}
	if sess.CloseDetail.Set {
		fmt.Printf("  close detail  : %s\n", sess.CloseDetail.Value)
	}
}
