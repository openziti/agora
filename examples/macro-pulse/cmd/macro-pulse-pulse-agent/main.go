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

		ping := func(ad api.Advertisement) {
			if len(ad.WorkgroupScopes) == 0 {
				a.Log().With("advertisement_id", ad.ID).Warnf("advertisement has no visible workgroup; skipping propose")
				return
			}
			sessCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
			defer cancel()
			sess, err := session.Propose(sessCtx, a, ad.ID, session.ProposeOptions{
				WorkgroupID: ad.WorkgroupScopes[0],
				Message:     "macro-pulse morning brief",
				Timeout:     10 * time.Second,
			})
			if err != nil {
				a.Log().
					With("advertisement_id", ad.ID).
					With("provider_account_id", ad.AccountId).
					Warnf("session propose failed: %v", err)
				return
			}
			logger := a.Log().
				With("session_id", sess.ID).
				With("tunnel_id", sess.TunnelID).
				With("advertisement_id", sess.AdvertisementID)
			if sess.ContractSnapshot != nil {
				logger = logger.
					With("contract_id", sess.ContractSnapshot.ContractId).
					With("contract_max_duration_seconds", sess.ContractSnapshot.MaxDurationSeconds).
					With("contract_max_envelope_bytes", sess.ContractSnapshot.MaxEnvelopeBytes).
					With("contract_access_mode", sess.ContractSnapshot.AccessMode)
			}
			logger.Infof("session active; sending ping envelope")

			messageType := firstCapabilityName(ad) + ".request"
			req := session.Envelope{
				MessageType: messageType,
				ContentType: "application/json",
				Payload:     []byte(`{"greeting":"good morning","source":"pulse-agent"}`),
			}
			if err := sess.Send(sessCtx, req); err != nil {
				logger.Warnf("send failed: %v", err)
				_ = sess.Close(ctx, "send failed")
				return
			}
			reply, err := sess.Receive(sessCtx)
			if err != nil {
				logger.Warnf("receive failed: %v", err)
				_ = sess.Close(ctx, "receive failed")
				return
			}
			logger.
				With("reply_message_type", reply.MessageType).
				With("reply_bytes", len(reply.Payload)).
				Infof("received reply envelope")
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
			ping(ad)
		}

		<-ctx.Done()
		return nil
	}); err != nil {
		os.Exit(1)
	}
}

func firstCapabilityName(ad api.Advertisement) string {
	if len(ad.Capabilities) == 0 {
		return "unknown"
	}
	return ad.Capabilities[0].Name
}
