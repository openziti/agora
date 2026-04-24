package main

import (
	"context"
	"fmt"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/internal/clioutput"
	"github.com/spf13/cobra"
)

func init() {
	sessionCmd.AddCommand(newSessionListCommand().cmd)
}

type sessionListCommand struct {
	states     []string
	role       string
	adID       string
	jsonOutput bool
	cmd        *cobra.Command
}

func newSessionListCommand() *sessionListCommand {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List sessions the caller participates in",
		Args:  cobra.NoArgs,
	}
	command := &sessionListCommand{cmd: cmd}
	cmd.Flags().StringSliceVar(&command.states, "state", nil, "Filter by session state (repeatable)")
	cmd.Flags().StringVar(&command.role, "role", "", "Filter by role: provider | consumer | both")
	cmd.Flags().StringVar(&command.adID, "advertisement", "", "Filter to a specific advertisement id")
	cmd.Flags().BoolVarP(&command.jsonOutput, "json", "j", false, "Output raw JSON")
	cmd.Run = command.run
	return command
}

func (cmd *sessionListCommand) run(_ *cobra.Command, _ []string) {
	root := requireEnabledRoot()
	client := openEnvironmentAPIClient(root)

	params := api.ListSessionsParams{}
	for _, st := range cmd.states {
		params.State = append(params.State, api.SessionState(st))
	}
	if cmd.role != "" {
		params.Role.SetTo(api.ListSessionsRole(cmd.role))
	}
	if cmd.adID != "" {
		params.AdvertisementId.SetTo(cmd.adID)
	}

	res, err := client.ListSessions(context.Background(), params)
	panicIfErr(err)
	listing, ok := res.(*api.ListSessionsResponse)
	if !ok {
		panic(fmt.Sprintf("unexpected list sessions response: %T", res))
	}

	if cmd.jsonOutput {
		panicIfErr(clioutput.RenderJSON(listing))
		return
	}

	t := clioutput.NewTable()
	t.AppendHeader(table.Row{"ID", "State", "Provider", "Consumer", "Workgroup", "Advertisement", "TunnelMode", "Proposed"})
	for _, s := range *listing {
		t.AppendRow(table.Row{
			s.ID,
			s.State,
			s.ProviderAccountId,
			s.ConsumerAccountId,
			s.WorkgroupId,
			s.AdvertisementId,
			s.TunnelMode,
			clioutput.TimeUTC(s.ProposedAt),
		})
	}
	clioutput.PrintTable(t)
	clioutput.PrintTotal("session(s)", len(*listing))
}
