package main

import (
	"context"
	"fmt"

	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/internal/clioutput"
	"github.com/spf13/cobra"
)

func init() {
	advertiseCmd.AddCommand(newAdvertiseDescribeCommand().cmd)
}

type advertiseDescribeCommand struct {
	jsonOutput bool
	cmd        *cobra.Command
}

func newAdvertiseDescribeCommand() *advertiseDescribeCommand {
	cmd := &cobra.Command{
		Use:   "describe <name|adv_...>",
		Short: "Describe an advertisement",
		Args:  cobra.ExactArgs(1),
	}
	command := &advertiseDescribeCommand{cmd: cmd}
	cmd.Flags().BoolVarP(&command.jsonOutput, "json", "j", false, "Output raw JSON")
	cmd.Run = command.run
	return command
}

func (cmd *advertiseDescribeCommand) run(_ *cobra.Command, args []string) {
	root := requireEnabledRoot()
	client := openEnvironmentAPIClient(root)
	advID := resolveAdvertisementID(client, args[0])

	res, err := client.GetAdvertisement(context.Background(), api.GetAdvertisementParams{AdvertisementId: advID})
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
		if c.Metadata.Set {
			for k, v := range c.Metadata.Value {
				fmt.Printf("      %s=%s\n", k, v)
			}
		}
	}
	fmt.Println("Interaction patterns:")
	for _, p := range ad.InteractionPatterns {
		if p.CustomPattern.Set {
			fmt.Printf("  - %s (%s)\n", p.Kind, p.CustomPattern.Value)
		} else {
			fmt.Printf("  - %s\n", p.Kind)
		}
	}
	fmt.Printf("Created at:          %s\n", clioutput.TimeUTC(ad.CreatedAt))
	fmt.Printf("Updated at:          %s\n", clioutput.TimeUTC(ad.UpdatedAt))
}
