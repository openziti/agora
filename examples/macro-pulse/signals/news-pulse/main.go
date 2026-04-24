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
	fs := flag.NewFlagSet("news-pulse", flag.ExitOnError)

	app := agent.New("news-pulse",
		agent.WithDescription("News volume and sentiment by topic (GDELT-backed)"),
		agent.WithFlagSet(fs),
		agent.WithRuntime(),
	)
	if err := app.Run(func(ctx context.Context, a *agent.Agent) error {
		ad, err := agentutil.EnsureAdvertisement(ctx, a.Controller(), agentutil.AdvertisementSpec{
			Name:        "news-pulse",
			Description: "News volume and sentiment by topic",
			Capabilities: []api.AdvertisementCapability{
				{Name: "signals.news", Description: api.NewOptString("News volume + tone per topic")},
			},
			InteractionPatterns: []api.AdvertisementInteractionPattern{{Kind: api.AdvertisementInteractionPatternKindRequestResponse}},
			WorkgroupNames:      []string{"signals-channel"},
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
