package main

import (
	"context"
	"fmt"

	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/internal/clioutput"
	"github.com/spf13/cobra"
)

func init() {
	catalogCmd.AddCommand(newCatalogDescribeCommand().cmd)
}

type catalogDescribeCommand struct {
	jsonOutput bool
	cmd        *cobra.Command
}

func newCatalogDescribeCommand() *catalogDescribeCommand {
	cmd := &cobra.Command{
		Use:   "describe <adv_...>",
		Short: "Describe an advertisement by ID (visibility-enforced)",
		Args:  cobra.ExactArgs(1),
	}
	command := &catalogDescribeCommand{cmd: cmd}
	cmd.Flags().BoolVarP(&command.jsonOutput, "json", "j", false, "Output raw JSON")
	cmd.Run = command.run
	return command
}

func (cmd *catalogDescribeCommand) run(_ *cobra.Command, args []string) {
	root := requireEnabledRoot()
	client := openEnvironmentAPIClient(root)

	res, err := client.GetAdvertisement(context.Background(), api.GetAdvertisementParams{AdvertisementId: args[0]})
	panicIfErr(err)
	ad, ok := res.(*api.Advertisement)
	if !ok {
		switch typed := res.(type) {
		case *api.GetAdvertisementNotFound:
			panic(typed.Message)
		case *api.GetAdvertisementUnauthorized:
			panic(typed.Message)
		case *api.GetAdvertisementInternalServerError:
			panic(typed.Message)
		}
		panic(fmt.Sprintf("unexpected get advertisement response: %T", res))
	}
	if cmd.jsonOutput {
		panicIfErr(clioutput.RenderJSON(ad))
		return
	}
	// Reuse the human-readable output from the advertise describe command.
	fmt.Printf("Name:                %s\n", ad.Name)
	fmt.Printf("ID:                  %s\n", ad.ID)
	fmt.Printf("Status:              %s\n", ad.Status)
	fmt.Printf("Owner account:       %s\n", ad.AccountId)
	fmt.Printf("Owner organization:  %s\n", ad.OrganizationId)
	if ad.Description.Set {
		fmt.Printf("Description:         %s\n", ad.Description.Value)
	}
	fmt.Printf("Workgroup scopes:    %v\n", ad.WorkgroupScopes)
	fmt.Println("Capabilities:")
	for _, c := range ad.Capabilities {
		fmt.Printf("  - %s", c.Name)
		if c.Description.Set {
			fmt.Printf(": %s", c.Description.Value)
		}
		fmt.Println()
	}
	fmt.Println("Interaction patterns:")
	for _, p := range ad.InteractionPatterns {
		if p.CustomPattern.Set {
			fmt.Printf("  - %s (%s)\n", p.Kind, p.CustomPattern.Value)
		} else {
			fmt.Printf("  - %s\n", p.Kind)
		}
	}
	fmt.Printf("Updated at:          %s\n", clioutput.TimeUTC(ad.UpdatedAt))
}
