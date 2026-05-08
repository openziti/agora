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
	fs := flag.NewFlagSet("weather-feed", flag.ExitOnError)

	app := agent.New("weather-feed",
		agent.WithDescription("Current conditions and short-horizon forecasts for economic hubs"),
		agent.WithFlagSet(fs),
		agent.WithRuntime(),
	)
	if err := app.Run(func(ctx context.Context, a *agent.Agent) error {
		ad, err := agentutil.EnsureAdvertisement(ctx, a, agentutil.AdvertisementSpec{
			Name:        "weather-feed",
			Description: "Current conditions and forecasts for economic hubs",
			Capabilities: []api.AdvertisementCapability{
				{Name: "weather.current", Description: api.NewOptString("Current conditions for configured cities")},
				{Name: "weather.forecast", Description: api.NewOptString("Short-horizon forecast for configured cities")},
			},
			InteractionPatterns: []api.AdvertisementInteractionPattern{{Kind: api.AdvertisementInteractionPatternKindRequestResponse}},
			WorkgroupNames:      []string{"weather-channel"},
			Contract: &agentutil.ContractSpec{
				Name:                "macro-pulse-provider-default",
				Description:         "Demo contract: bounded session duration for macro-pulse morning briefings.",
				MaxDurationSeconds:  60,
				AllowedMessageTypes: []string{"*"},
				AccessMode:          api.ContractAccessModeApprovalRequired,
			},
		})
		if err != nil {
			a.Log().Errorf("publish advertisement: %v", err)
			return err
		}
		a.Log().With("advertisement_id", ad.ID).Infof("alive; advertisement published")
		handler := &agentutil.WeatherRouterHandler{Agent: a, Live: a.Live()}
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
