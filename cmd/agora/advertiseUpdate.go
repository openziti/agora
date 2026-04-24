package main

import (
	"context"
	"fmt"

	"github.com/openziti/agora/internal/api"
	"github.com/spf13/cobra"
)

func init() {
	advertiseCmd.AddCommand(newAdvertiseUpdateCommand().cmd)
}

type advertiseUpdateCommand struct {
	name         string
	description  string
	capabilities []string
	interactions []string
	workgroups   []string
	cmd          *cobra.Command
}

func newAdvertiseUpdateCommand() *advertiseUpdateCommand {
	cmd := &cobra.Command{
		Use:   "update <name|adv_...>",
		Short: "Update an existing advertisement",
		Args:  cobra.ExactArgs(1),
	}
	command := &advertiseUpdateCommand{cmd: cmd}
	cmd.Flags().StringVar(&command.name, "name", "", "Optional new name for the advertisement")
	cmd.Flags().StringVar(&command.description, "description", "", "Optional new description")
	cmd.Flags().StringSliceVar(&command.capabilities, "capability", nil, "Capability spec; repeatable; replaces current set when provided")
	cmd.Flags().StringSliceVar(&command.interactions, "interaction", nil, "Interaction pattern; repeatable; replaces current set when provided")
	cmd.Flags().StringSliceVar(&command.workgroups, "workgroup", nil, "Workgroup name or wg_...; repeatable; replaces current set when provided")
	cmd.Run = command.run
	return command
}

func (cmd *advertiseUpdateCommand) run(cobraCmd *cobra.Command, args []string) {
	root := requireEnabledRoot()
	client := openEnvironmentAPIClient(root)
	advID := resolveAdvertisementID(client, args[0])

	req := &api.UpdateAdvertisementRequest{}
	if cobraCmd.Flags().Changed("name") {
		req.Name.SetTo(cmd.name)
	}
	if cobraCmd.Flags().Changed("description") {
		req.Description.SetTo(cmd.description)
	}
	if cobraCmd.Flags().Changed("capability") {
		caps := make([]api.AdvertisementCapability, 0, len(cmd.capabilities))
		for _, raw := range cmd.capabilities {
			caps = append(caps, parseCapability(raw))
		}
		req.Capabilities = caps
	}
	if cobraCmd.Flags().Changed("interaction") {
		patterns := make([]api.AdvertisementInteractionPattern, 0, len(cmd.interactions))
		for _, raw := range cmd.interactions {
			patterns = append(patterns, parseInteractionPattern(raw))
		}
		req.InteractionPatterns = patterns
	}
	if cobraCmd.Flags().Changed("workgroup") {
		scopes := make([]string, 0, len(cmd.workgroups))
		for _, wg := range cmd.workgroups {
			scopes = append(scopes, resolveWorkgroupID(client, wg))
		}
		req.WorkgroupScopes = scopes
	}

	res, err := client.UpdateAdvertisement(context.Background(), req, api.UpdateAdvertisementParams{AdvertisementId: advID})
	panicIfErr(err)
	switch typed := res.(type) {
	case *api.Advertisement:
		fmt.Printf("updated advertisement '%s'\n", typed.ID)
	case *api.UpdateAdvertisementBadRequest:
		panic(typed.Message)
	case *api.UpdateAdvertisementForbidden:
		panic(typed.Message)
	case *api.UpdateAdvertisementNotFound:
		panic(typed.Message)
	case *api.UpdateAdvertisementConflict:
		panic(typed.Message)
	case *api.UpdateAdvertisementUnauthorized:
		panic(typed.Message)
	case *api.UpdateAdvertisementInternalServerError:
		panic(typed.Message)
	default:
		panic(fmt.Sprintf("unexpected update advertisement response: %T", res))
	}
}
