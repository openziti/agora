package main

import (
	"context"
	"flag"
	"os"

	"github.com/openziti/agora/examples/macro-pulse/internal/agentutil"
	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/sdk/agent"
	"github.com/openziti/agora/sdk/agent/session"
)

func main() {
	fs := flag.NewFlagSet("search-trends", flag.ExitOnError)

	app := agent.New("search-trends",
		agent.WithDescription("Relative search-interest signals (via Wikipedia pageview volume)"),
		agent.WithFlagSet(fs),
		agent.WithRuntime(),
	)
	if err := app.Run(func(ctx context.Context, a *agent.Agent) error {
		ad, err := agentutil.EnsureAdvertisement(ctx, a.Controller(), agentutil.AdvertisementSpec{
			Name:        "search-trends",
			Description: "Relative search-interest signals (via Wikipedia pageview volume)",
			Capabilities: []api.AdvertisementCapability{
				{Name: "signals.search", Description: api.NewOptString("Relative search interest per tracked term")},
			},
			InteractionPatterns: []api.AdvertisementInteractionPattern{{Kind: api.AdvertisementInteractionPatternKindRequestResponse}},
			WorkgroupNames:      []string{"signals-channel"},
			Contract: &agentutil.ContractSpec{
				Name:               "macro-pulse-provider-default",
				Description:        "Demo contract: bounded session duration for macro-pulse morning briefings.",
				MaxDurationSeconds:  60,
				AllowedMessageTypes: []string{"*"},
				AccessMode:         api.ContractAccessModeApprovalRequired,
			},
		})
		if err != nil {
			a.Log().Errorf("publish advertisement: %v", err)
			return err
		}
		a.Log().With("advertisement_id", ad.ID).Infof("alive; advertisement published")
		handler := &agentutil.EchoSessionHandler{Agent: a}
		go func() {
			if err := session.RegisterHandler(ctx, a, ad.ID, handler); err != nil {
				a.Log().With("advertisement_id", ad.ID).Warnf("session handler exited: %v", err)
			}
		}()
		<-ctx.Done()
		return nil
	}); err != nil {
		os.Exit(1)
	}
}
