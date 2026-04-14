package controller

import (
	"context"
	"time"

	"github.com/michaelquigley/df/dl"
	"github.com/openziti/agora/internal/persistence"
)

const (
	tunnelServeLeaseTTL     = 45 * time.Second
	tunnelServeReapInterval = 15 * time.Second
)

func (s *Service) RunTunnelServeReaper(ctx context.Context) {
	ticker := time.NewTicker(tunnelServeReapInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			if err := s.ReapStaleTunnelServes(ctx, now.UTC()); err != nil {
				dl.Errorf("tunnel serve reaper failed: %v", err)
			}
		}
	}
}

func (s *Service) ReapStaleTunnelServes(ctx context.Context, now time.Time) error {
	expired, err := s.store.TunnelServes.ListExpiredActive(ctx, s.store.DB(), now.Add(-tunnelServeLeaseTTL))
	if err != nil {
		return err
	}
	if len(expired) == 0 {
		return nil
	}

	for i := range expired {
		serve := expired[i]
		disconnectedAt := now
		if err := s.store.TunnelServes.UpdateState(ctx, s.store.DB(), serve.ID, persistence.TunnelServeStateStale, &disconnectedAt); err != nil {
			return err
		}
		dl.Infof("reaped stale tunnel serve serve_id='%s' tunnel_id='%s' account_id='%s' environment_id='%s'", serve.ID, serve.TunnelID, serve.AccountID, serve.EnvironmentID)
	}

	return nil
}
