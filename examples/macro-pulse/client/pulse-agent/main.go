package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/sdk/agent"
	"github.com/openziti/agora/sdk/agent/session"
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

		propose := func(ad api.Advertisement) {
			if len(ad.WorkgroupScopes) == 0 {
				a.Log().With("advertisement_id", ad.ID).Warnf("advertisement has no visible workgroup; skipping propose")
				return
			}
			sess, err := session.Propose(ctx, a, ad.ID, session.ProposeOptions{
				WorkgroupID: ad.WorkgroupScopes[0],
				Message:     "macro-pulse morning brief",
				Timeout:     15 * time.Second,
			})
			if err != nil {
				a.Log().
					With("advertisement_id", ad.ID).
					With("provider_account_id", ad.AccountId).
					Warnf("session propose failed: %v", err)
				return
			}
			a.Log().
				With("session_id", sess.ID).
				With("tunnel_id", sess.TunnelID).
				With("advertisement_id", sess.AdvertisementID).
				Infof("session active; closing")
			if err := sess.Close(ctx, "brief complete"); err != nil {
				a.Log().With("session_id", sess.ID).Warnf("close session failed: %v", err)
			}
		}

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
			propose(ad)
		}

		<-ctx.Done()
		return nil
	}); err != nil {
		os.Exit(1)
	}
}
