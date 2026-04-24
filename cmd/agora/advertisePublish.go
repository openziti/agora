package main

import (
	"context"
	"fmt"

	"github.com/openziti/agora/internal/api"
	"github.com/spf13/cobra"
)

func init() {
	advertiseCmd.AddCommand(newAdvertisePublishCommand().cmd)
}

type advertisePublishCommand struct {
	description  string
	capabilities []string
	interactions []string
	workgroups   []string
	tunnelMode   string
	cmd          *cobra.Command
}

func newAdvertisePublishCommand() *advertisePublishCommand {
	cmd := &cobra.Command{
		Use:   "publish <name>",
		Short: "Publish an advertisement",
		Args:  cobra.ExactArgs(1),
	}
	command := &advertisePublishCommand{cmd: cmd}
	cmd.Flags().StringVar(&command.description, "description", "", "Optional human-readable description")
	cmd.Flags().StringSliceVar(&command.capabilities, "capability", nil, "Capability spec: name[=description][:k1=v1,k2=v2]; repeatable; required")
	cmd.Flags().StringSliceVar(&command.interactions, "interaction", nil, "Interaction pattern: request-response | stream | broadcast | custom:<pattern>; repeatable; required")
	cmd.Flags().StringSliceVar(&command.workgroups, "workgroup", nil, "Workgroup name or wg_... id; repeatable; required")
	cmd.Flags().StringVar(&command.tunnelMode, "tunnel-mode", "", "Tunnel mode for sessions backed by this advertisement: http|tcp|udp (default tcp)")
	panicIfErr(cmd.MarkFlagRequired("capability"))
	panicIfErr(cmd.MarkFlagRequired("interaction"))
	panicIfErr(cmd.MarkFlagRequired("workgroup"))
	cmd.Run = command.run
	return command
}

func (cmd *advertisePublishCommand) run(_ *cobra.Command, args []string) {
	root := requireEnabledRoot()
	client := openEnvironmentAPIClient(root)

	caps := make([]api.AdvertisementCapability, 0, len(cmd.capabilities))
	for _, raw := range cmd.capabilities {
		caps = append(caps, parseCapability(raw))
	}
	patterns := make([]api.AdvertisementInteractionPattern, 0, len(cmd.interactions))
	for _, raw := range cmd.interactions {
		patterns = append(patterns, parseInteractionPattern(raw))
	}
	scopes := make([]string, 0, len(cmd.workgroups))
	for _, wg := range cmd.workgroups {
		scopes = append(scopes, resolveWorkgroupID(client, wg))
	}

	req := &api.PublishAdvertisementRequest{
		Name:                args[0],
		Capabilities:        caps,
		InteractionPatterns: patterns,
		WorkgroupScopes:     scopes,
	}
	if cmd.description != "" {
		req.Description.SetTo(cmd.description)
	}
	if cmd.tunnelMode != "" {
		req.TunnelMode.SetTo(api.AdvertisementTunnelMode(cmd.tunnelMode))
	}

	res, err := client.PublishAdvertisement(context.Background(), req)
	panicIfErr(err)
	switch typed := res.(type) {
	case *api.Advertisement:
		fmt.Printf("published advertisement '%s' (id=%s)\n", typed.Name, typed.ID)
	case *api.PublishAdvertisementBadRequest:
		panic(typed.Message)
	case *api.PublishAdvertisementForbidden:
		panic(typed.Message)
	case *api.PublishAdvertisementConflict:
		panic(typed.Message)
	case *api.PublishAdvertisementUnauthorized:
		panic(typed.Message)
	case *api.PublishAdvertisementInternalServerError:
		panic(typed.Message)
	default:
		panic(fmt.Sprintf("unexpected publish advertisement response: %T", res))
	}
}
