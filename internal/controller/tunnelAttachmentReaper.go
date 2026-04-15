package controller

import (
	"context"
	"time"

	"github.com/michaelquigley/df/dl"
	"github.com/openziti/agora/internal/fabric/openziti/automation"
	"github.com/openziti/agora/internal/persistence"
)

const (
	tunnelAttachmentLeaseTTL     = 45 * time.Second
	tunnelAttachmentReapInterval = 15 * time.Second
)

func (s *Service) RunTunnelAttachmentReaper(ctx context.Context) {
	ticker := time.NewTicker(tunnelAttachmentReapInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			if err := s.ReapStaleTunnelAttachments(ctx, now.UTC()); err != nil {
				dl.Errorf("tunnel attachment reaper failed: %v", err)
			}
		}
	}
}

func (s *Service) ReapStaleTunnelAttachments(ctx context.Context, now time.Time) error {
	expired, err := s.store.TunnelAttachments.ListExpiredActive(ctx, s.store.DB(), now.Add(-tunnelAttachmentLeaseTTL))
	if err != nil {
		return err
	}
	if len(expired) == 0 {
		return nil
	}

	_, tunnelLifecycle, err := s.lifecycleFactory(ctx)
	if err != nil {
		return err
	}

	for i := range expired {
		attachment := expired[i]
		if attachment.DialPolicyID != nil {
			if err := tunnelLifecycle.Deprovision(ctx, automation.DeprovisionTunnelSpec{DialPolicyID: *attachment.DialPolicyID}); err != nil {
				return err
			}
		}
		disconnectedAt := now
		if err := s.store.TunnelAttachments.UpdateState(ctx, s.store.DB(), attachment.ID, persistence.TunnelAttachmentStateStale, &disconnectedAt); err != nil {
			return err
		}
		dl.Infof("reaped stale tunnel attachment attachment_id='%s' tunnel_id='%s' account_id='%s' environment_id='%s'", attachment.ID, attachment.TunnelID, attachment.AccountID, attachment.EnvironmentID)
	}

	return nil
}
