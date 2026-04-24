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
	fs := flag.NewFlagSet("correlator", flag.ExitOnError)

	app := agent.New("correlator",
		agent.WithDescription("Pearson correlation coefficient between two aligned time series"),
		agent.WithFlagSet(fs),
		agent.WithRuntime(),
	)
	if err := app.Run(func(ctx context.Context, a *agent.Agent) error {
		ad, err := agentutil.EnsureAdvertisement(ctx, a.Controller(), agentutil.AdvertisementSpec{
			Name:        "correlator",
			Description: "Pearson correlation between two time series",
			Capabilities: []api.AdvertisementCapability{
				{Name: "analytics.correlate", Description: api.NewOptString("Compute Pearson r between aligned series")},
			},
			InteractionPatterns: []api.AdvertisementInteractionPattern{{Kind: api.AdvertisementInteractionPatternKindRequestResponse}},
			WorkgroupNames:      []string{"analytics-channel"},
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
