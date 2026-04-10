package controller

import (
	"context"
	"errors"
	"time"

	"github.com/michaelquigley/df/dl"
	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/internal/openziti/automation"
	"github.com/openziti/agora/internal/persistence"
)

func (s *Service) RemoveTunnelGrant(ctx context.Context, params api.RemoveTunnelGrantParams) (api.RemoveTunnelGrantRes, error) {
	principal, err := requireAccountPrincipal(ctx)
	if err != nil {
		dl.Warnf("unauthorized remove tunnel grant request tunnel_id='%s' account_id='%s'", params.TunnelId, params.AccountId)
		return &api.RemoveTunnelGrantUnauthorized{Code: "unauthorized", Message: "unauthorized"}, nil
	}
	dl.Infof("removing tunnel grant tunnel_id='%s' account_id='%s' %s", params.TunnelId, params.AccountId, principalLogFields(principal))

	tunnel, err := s.requireManagedTunnel(ctx, principal, params.TunnelId)
	if err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			return &api.RemoveTunnelGrantNotFound{Code: "not_found", Message: "tunnel not found"}, nil
		}
		return &api.RemoveTunnelGrantInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}

	if err := s.store.TunnelGrants.Delete(ctx, s.store.DB(), tunnel.ID, params.AccountId, tunnel.OrganizationID); err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			return &api.RemoveTunnelGrantNotFound{Code: "not_found", Message: "grant not found"}, nil
		}
		return &api.RemoveTunnelGrantInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}

	attachments, err := s.store.TunnelAttachments.ListByTunnelAndAccount(ctx, s.store.DB(), tunnel.ID, tunnel.OrganizationID, params.AccountId)
	if err != nil {
		return &api.RemoveTunnelGrantInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}

	_, tunnelLifecycle, err := s.lifecycleFactory(ctx)
	if err != nil {
		return &api.RemoveTunnelGrantInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}

	disconnectedAt := time.Now().UTC()
	for i := range attachments {
		attachment := attachments[i]
		if attachment.DialPolicyID != nil {
			if err := tunnelLifecycle.Deprovision(ctx, automation.DeprovisionTunnelSpec{DialPolicyID: *attachment.DialPolicyID}); err != nil {
				dl.Errorf("remove tunnel grant deprovision failed attachment_id='%s' tunnel_id='%s' %s: %v", attachment.ID, tunnel.ID, principalLogFields(principal), err)
				return &api.RemoveTunnelGrantInternalServerError{Code: "internal_error", Message: err.Error()}, nil
			}
		}
		if err := s.store.TunnelAttachments.UpdateState(ctx, s.store.DB(), attachment.ID, persistence.TunnelAttachmentStateDisconnected, &disconnectedAt); err != nil {
			return &api.RemoveTunnelGrantInternalServerError{Code: "internal_error", Message: err.Error()}, nil
		}
	}

	return &api.RemoveTunnelGrantNoContent{}, nil
}
