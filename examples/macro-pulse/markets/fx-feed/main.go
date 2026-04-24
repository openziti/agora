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
	fs := flag.NewFlagSet("fx-feed", flag.ExitOnError)

	app := agent.New("fx-feed",
		agent.WithDescription("Major currency pair spot rates and recent history"),
		agent.WithFlagSet(fs),
		agent.WithRuntime(),
	)
	if err := app.Run(func(ctx context.Context, a *agent.Agent) error {
		ad, err := agentutil.EnsureAdvertisement(ctx, a.Controller(), agentutil.AdvertisementSpec{
			Name:        "fx-feed",
			Description: "Major currency pair spot rates and recent history",
			Capabilities: []api.AdvertisementCapability{
				{Name: "markets.fx", Description: api.NewOptString("Spot FX rates for major pairs"), Metadata: api.NewOptAdvertisementCapabilityMetadata(api.AdvertisementCapabilityMetadata{"asset_class": "fx"})},
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
