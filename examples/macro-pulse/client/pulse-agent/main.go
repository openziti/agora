package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/sdk/agent"
)

func main() {
	fs := flag.NewFlagSet("pulse-agent", flag.ExitOnError)

	app := agent.New("pulse-agent",
		agent.WithDescription("Macro Pulse orchestrator: composes across providers and tools to produce the morning brief"),
		agent.WithFlagSet(fs),
		agent.WithRuntime(),
	)
	if err := app.Run(func(ctx context.Context, a *agent.Agent) error {
		res, err := a.Controller().SearchCatalog(ctx, api.SearchCatalogParams{})
		if err != nil {
			a.Log().Errorf("catalog search failed: %v", err)
			return err
		}
		resp, ok := res.(*api.CatalogSearchResponse)
		if !ok {
			a.Log().Errorf("unexpected catalog search response: %T", res)
			return fmt.Errorf("unexpected catalog search response: %T", res)
		}
		a.Log().With("count", len(resp.Items)).Infof("discovered advertisements via catalog")
		for _, ad := range resp.Items {
			capNames := make([]string, 0, len(ad.Capabilities))
			for _, c := range ad.Capabilities {
				capNames = append(capNames, c.Name)
			}
			a.Log().
				With("advertisement_id", ad.ID).
				With("name", ad.Name).
				With("owner_account_id", ad.AccountId).
				With("capabilities", capNames).
				Infof("catalog entry")
		}
		<-ctx.Done()
		return nil
	}); err != nil {
		os.Exit(1)
	}
}
