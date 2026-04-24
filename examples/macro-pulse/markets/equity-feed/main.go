package main

import (
	"context"
	"flag"
	"os"

	"github.com/openziti/agora/examples/macro-pulse/internal/agentutil"
	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/sdk/agent"
)

func main() {
	fs := flag.NewFlagSet("equity-feed", flag.ExitOnError)

	app := agent.New("equity-feed",
		agent.WithDescription("Major US equity indices and sector ETFs"),
		agent.WithFlagSet(fs),
		agent.WithRuntime(),
	)
	if err := app.Run(func(ctx context.Context, a *agent.Agent) error {
		ad, err := agentutil.EnsureAdvertisement(ctx, a.Controller(), agentutil.AdvertisementSpec{
			Name:        "equity-feed",
			Description: "Major US equity indices and sector ETFs",
			Capabilities: []api.AdvertisementCapability{
				{Name: "markets.equity", Description: api.NewOptString("S&P 500 and major sector ETFs"), Metadata: api.NewOptAdvertisementCapabilityMetadata(api.AdvertisementCapabilityMetadata{"asset_class": "equity"})},
			},
			InteractionPatterns: []api.AdvertisementInteractionPattern{{Kind: api.AdvertisementInteractionPatternKindRequestResponse}},
			WorkgroupNames:      []string{"markets-channel"},
		})
		if err != nil {
			a.Log().Errorf("publish advertisement: %v", err)
			return err
		}
		a.Log().With("advertisement_id", ad.ID).Infof("alive; advertisement published")
		<-ctx.Done()
		return nil
	}); err != nil {
		os.Exit(1)
	}
}
