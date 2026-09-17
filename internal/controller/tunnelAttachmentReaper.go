package controller

import (
	"context"
	"time"

	"github.com/michaelquigley/df/dl"
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
	expiredBefore := now.Add(-tunnelAttachmentLeaseTTL)
	expired, err := s.store.TunnelAttachments.ListExpiredActive(ctx, s.store.DB(), expiredBefore)
	if err != nil {
		return err
	}
	if len(expired) == 0 {
		return nil
	}

	for i := range expired {
		attachment := expired[i]
		reaped, err := s.reapStaleTunnelAttachment(ctx, attachment, expiredBefore)
		if err != nil {
			return err
		}
		if !reaped {
			continue
		}
		dl.Infof("reaped stale tunnel attachment attachment_id='%s' tunnel_id='%s' account_id='%s' environment_id='%s'", attachment.ID, attachment.TunnelID, attachment.AccountID, attachment.EnvironmentID)
	}

	return nil
}

func (s *Service) reapStaleTunnelAttachment(ctx context.Context, attachment persistence.TunnelAttachment, expiredBefore time.Time) (bool, error) {
	return s.store.TunnelAttachments.MarkStaleIfExpired(ctx, s.store.DB(), attachment.ID, expiredBefore)
}
