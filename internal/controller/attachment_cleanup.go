package controller

import (
	"context"
	"fmt"
	"time"

	"github.com/openziti/agora/internal/fabric/openziti/automation"
	"github.com/openziti/agora/internal/persistence"
)

func uniqueTunnelAttachments(groups ...[]persistence.TunnelAttachment) []persistence.TunnelAttachment {
	seen := map[string]struct{}{}
	result := []persistence.TunnelAttachment{}
	for _, group := range groups {
		for i := range group {
			attachment := group[i]
			if _, ok := seen[attachment.ID]; ok {
				continue
			}
			seen[attachment.ID] = struct{}{}
			result = append(result, attachment)
		}
	}
	return result
}

func tunnelAttachmentIDs(attachments []persistence.TunnelAttachment) []string {
	ids := make([]string, 0, len(attachments))
	for i := range attachments {
		ids = append(ids, attachments[i].ID)
	}
	return ids
}

func deprovisionAttachmentPolicies(ctx context.Context, tunnelLifecycle tunnelLifecycle, attachments []persistence.TunnelAttachment) error {
	for i := range attachments {
		attachment := attachments[i]
		if attachment.DialPolicyID == nil {
			continue
		}
		if err := tunnelLifecycle.Deprovision(ctx, automation.DeprovisionTunnelSpec{DialPolicyID: *attachment.DialPolicyID}); err != nil {
			return fmt.Errorf("deprovision attachment '%s': %w", attachment.ID, err)
		}
	}
	return nil
}

func (s *Service) detachAndSoftDeleteAttachments(ctx context.Context, tx persistence.Queryer, attachments []persistence.TunnelAttachment, state persistence.TunnelAttachmentState, disconnectedAt time.Time) error {
	attachments = uniqueTunnelAttachments(attachments)
	for i := range attachments {
		if err := s.detachTunnel(ctx, tx, attachments[i], state, &disconnectedAt); err != nil {
			return err
		}
	}
	return s.store.TunnelAttachments.SoftDeleteByIDs(ctx, tx, tunnelAttachmentIDs(attachments))
}
