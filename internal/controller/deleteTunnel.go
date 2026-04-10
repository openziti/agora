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

func (s *Service) DeleteTunnel(ctx context.Context, params api.DeleteTunnelParams) (api.DeleteTunnelRes, error) {
	principal, err := requireAccountPrincipal(ctx)
	if err != nil {
		dl.Warnf("unauthorized delete tunnel request tunnel_id='%s'", params.TunnelId)
		return &api.DeleteTunnelUnauthorized{Code: "unauthorized", Message: "unauthorized"}, nil
	}
	dl.Infof("deleting tunnel tunnel_id='%s' %s", params.TunnelId, principalLogFields(principal))

	tunnel, err := s.requireManagedTunnel(ctx, principal, params.TunnelId)
	if err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			dl.Warnf("delete tunnel not found tunnel_id='%s' %s", params.TunnelId, principalLogFields(principal))
			return &api.DeleteTunnelNotFound{Code: "not_found", Message: "tunnel not found"}, nil
		}
		dl.Errorf("delete tunnel lookup failed tunnel_id='%s' %s: %v", params.TunnelId, principalLogFields(principal), err)
		return &api.DeleteTunnelInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}

	attachments, err := s.store.TunnelAttachments.ListByTunnel(ctx, s.store.DB(), tunnel.ID, tunnel.OrganizationID)
	if err != nil {
		dl.Errorf("delete tunnel attachment lookup failed tunnel_id='%s' %s: %v", tunnel.ID, principalLogFields(principal), err)
		return &api.DeleteTunnelInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}

	_, tunnelLifecycle, err := s.lifecycleFactory(ctx)
	if err != nil {
		dl.Errorf("delete tunnel lifecycle initialization failed tunnel_id='%s' %s: %v", tunnel.ID, principalLogFields(principal), err)
		return &api.DeleteTunnelInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}

	for i := range attachments {
		attachment := attachments[i]
		if attachment.DialPolicyID != nil {
			if err := tunnelLifecycle.Deprovision(ctx, automation.DeprovisionTunnelSpec{DialPolicyID: *attachment.DialPolicyID}); err != nil {
				dl.Errorf("delete tunnel attachment deprovision failed attachment_id='%s' tunnel_id='%s' %s: %v", attachment.ID, tunnel.ID, principalLogFields(principal), err)
				return &api.DeleteTunnelInternalServerError{Code: "internal_error", Message: err.Error()}, nil
			}
		}
	}

	if err := tunnelLifecycle.Deprovision(ctx, automation.DeprovisionTunnelSpec{
		ServiceID:                 optionalStringValue(tunnel.ZitiServiceID),
		BindPolicyID:              optionalStringValue(tunnel.BindPolicyID),
		ServiceEdgeRouterPolicyID: optionalStringValue(tunnel.ServiceEdgeRouterPolicyID),
	}); err != nil {
		dl.Errorf("delete tunnel deprovision failed tunnel_id='%s' %s: %v", tunnel.ID, principalLogFields(principal), err)
		return &api.DeleteTunnelInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}

	disconnectedAt := time.Now().UTC()
	if err := s.store.WithTx(ctx, func(tx persistence.Queryer) error {
		for i := range attachments {
			if err := s.store.TunnelAttachments.UpdateState(ctx, tx, attachments[i].ID, persistence.TunnelAttachmentStateDisconnected, &disconnectedAt); err != nil {
				return err
			}
		}
		if err := s.store.TunnelAttachments.DeleteByTunnel(ctx, tx, tunnel.ID, tunnel.OrganizationID); err != nil {
			return err
		}
		if err := s.store.TunnelGrants.DeleteByTunnel(ctx, tx, tunnel.ID, tunnel.OrganizationID); err != nil {
			return err
		}
		return s.store.Tunnels.Delete(ctx, tx, tunnel.ID, tunnel.OrganizationID)
	}); err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			return &api.DeleteTunnelNotFound{Code: "not_found", Message: "tunnel not found"}, nil
		}
		dl.Errorf("delete tunnel persistence failed tunnel_id='%s' %s: %v", tunnel.ID, principalLogFields(principal), err)
		return &api.DeleteTunnelInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}

	dl.Infof("deleted tunnel tunnel_id='%s' %s", tunnel.ID, principalLogFields(principal))
	return &api.DeleteTunnelNoContent{}, nil
}
