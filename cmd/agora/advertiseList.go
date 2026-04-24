package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/internal/clioutput"
	"github.com/spf13/cobra"
)

func init() {
	advertiseCmd.AddCommand(newAdvertiseListCommand().cmd)
}

type advertiseListCommand struct {
	status     string
	jsonOutput bool
	cmd        *cobra.Command
}

func newAdvertiseListCommand() *advertiseListCommand {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List advertisements published by the current account",
		Args:  cobra.NoArgs,
	}
	command := &advertiseListCommand{cmd: cmd}
	cmd.Flags().StringVar(&command.status, "status", "", "Filter by status: active or retracted")
	cmd.Flags().BoolVarP(&command.jsonOutput, "json", "j", false, "Output raw JSON")
	cmd.Run = command.run
	return command
}

func (cmd *advertiseListCommand) run(_ *cobra.Command, _ []string) {
	root := requireEnabledRoot()
	client := openEnvironmentAPIClient(root)

	params := api.ListAdvertisementsParams{}
	if cmd.status != "" {
		params.Status.SetTo(api.AdvertisementStatus(cmd.status))
	}

	res, err := client.ListAdvertisements(context.Background(), params)
	panicIfErr(err)
	listing, ok := res.(*api.ListAdvertisementsResponse)
	if !ok {
		switch typed := res.(type) {
		case *api.ListAdvertisementsUnauthorized:
			panic(typed.Message)
		case *api.ListAdvertisementsInternalServerError:
			panic(typed.Message)
		}
		panic(fmt.Sprintf("unexpected list advertisements response: %T", res))
	}

	if cmd.jsonOutput {
		panicIfErr(clioutput.RenderJSON(listing))
		return
	}

	t := clioutput.NewTable()
	t.AppendHeader(table.Row{"Name", "ID", "Status", "Workgroups", "Capabilities", "Updated"})
	for _, ad := range *listing {
		capNames := make([]string, 0, len(ad.Capabilities))
		for _, c := range ad.Capabilities {
			capNames = append(capNames, c.Name)
		}
		t.AppendRow(table.Row{
			ad.Name,
			ad.ID,
			ad.Status,
			strings.Join(ad.WorkgroupScopes, ","),
			strings.Join(capNames, ","),
			clioutput.TimeUTC(ad.UpdatedAt),
		})
	}
	clioutput.PrintTable(t)
	clioutput.PrintTotal("advertisement(s)", len(*listing))
}
