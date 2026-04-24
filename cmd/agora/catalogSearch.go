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
	catalogCmd.AddCommand(newCatalogSearchCommand().cmd)
}

type catalogSearchCommand struct {
	workgroups   []string
	capability   string
	interactions []string
	ownerOrg     string
	limit        int
	cursor       string
	jsonOutput   bool
	cmd          *cobra.Command
}

func newCatalogSearchCommand() *catalogSearchCommand {
	cmd := &cobra.Command{
		Use:   "search",
		Short: "Search the catalog for visible advertisements",
		Args:  cobra.NoArgs,
	}
	command := &catalogSearchCommand{cmd: cmd}
	cmd.Flags().StringSliceVar(&command.workgroups, "workgroup", nil, "Filter by workgroup name or wg_... id; repeatable")
	cmd.Flags().StringVar(&command.capability, "capability", "", "Substring match on capability name (case-insensitive)")
	cmd.Flags().StringSliceVar(&command.interactions, "interaction", nil, "Filter by interaction pattern kind; repeatable")
	cmd.Flags().StringVar(&command.ownerOrg, "owner-org", "", "Filter by owning organization id (org_...)")
	cmd.Flags().IntVar(&command.limit, "limit", 0, "Page size (default 50, max 200)")
	cmd.Flags().StringVar(&command.cursor, "cursor", "", "Continue from a previous response's nextCursor")
	cmd.Flags().BoolVarP(&command.jsonOutput, "json", "j", false, "Output raw JSON")
	cmd.Run = command.run
	return command
}

func (cmd *catalogSearchCommand) run(_ *cobra.Command, _ []string) {
	root := requireEnabledRoot()
	client := openEnvironmentAPIClient(root)

	params := api.SearchCatalogParams{}
	if len(cmd.workgroups) > 0 {
		resolved := make([]string, 0, len(cmd.workgroups))
		for _, wg := range cmd.workgroups {
			resolved = append(resolved, resolveWorkgroupID(client, wg))
		}
		params.Workgroup = resolved
	}
	if cmd.capability != "" {
		params.Capability.SetTo(cmd.capability)
	}
	if len(cmd.interactions) > 0 {
		kinds := make([]api.AdvertisementInteractionPatternKind, 0, len(cmd.interactions))
		for _, k := range cmd.interactions {
			kinds = append(kinds, api.AdvertisementInteractionPatternKind(k))
		}
		params.InteractionPattern = kinds
	}
	if cmd.ownerOrg != "" {
		params.OwnerOrganizationId.SetTo(cmd.ownerOrg)
	}
	if cmd.limit > 0 {
		params.Limit.SetTo(cmd.limit)
	}
	if cmd.cursor != "" {
		params.Cursor.SetTo(cmd.cursor)
	}

	res, err := client.SearchCatalog(context.Background(), params)
	panicIfErr(err)
	resp, ok := res.(*api.CatalogSearchResponse)
	if !ok {
		switch typed := res.(type) {
		case *api.SearchCatalogBadRequest:
			panic(typed.Message)
		case *api.SearchCatalogUnauthorized:
			panic(typed.Message)
		case *api.SearchCatalogInternalServerError:
			panic(typed.Message)
		}
		panic(fmt.Sprintf("unexpected search catalog response: %T", res))
	}

	if cmd.jsonOutput {
		panicIfErr(clioutput.RenderJSON(resp))
		return
	}

	t := clioutput.NewTable()
	t.AppendHeader(table.Row{"Name", "ID", "Owner", "Workgroups", "Capabilities", "Updated"})
	for _, ad := range resp.Items {
		capNames := make([]string, 0, len(ad.Capabilities))
		for i, c := range ad.Capabilities {
			if i >= 3 {
				capNames = append(capNames, "...")
				break
			}
			capNames = append(capNames, c.Name)
		}
		t.AppendRow(table.Row{
			ad.Name,
			ad.ID,
			ad.AccountId,
			strings.Join(ad.WorkgroupScopes, ","),
			strings.Join(capNames, ","),
			clioutput.TimeUTC(ad.UpdatedAt),
		})
	}
	clioutput.PrintTable(t)
	clioutput.PrintTotal("advertisement(s)", len(resp.Items))
	if resp.NextCursor.Set {
		fmt.Printf("nextCursor: %s\n", resp.NextCursor.Value)
	}
}
