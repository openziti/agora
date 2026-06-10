package controller

import (
	"context"
	"errors"
	"time"

	"github.com/michaelquigley/df/dl"
	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/internal/fabric/openziti/automation"
	"github.com/openziti/agora/internal/persistence"
)

func (s *Service) DeleteTunnel(ctx context.Context, params api.DeleteTunnelParams) (api.DeleteTunnelRes, error) {
	principal, err := requireAccountPrincipal(ctx)
	if err != nil {
		dl.Warnf("unauthorized delete tunnel request tunnel_id='%s'", params.TunnelId)
		return &api.DeleteTunnelUnauthorized{Code: "unauthorized", Message: "unauthorized"}, nil
	}
	dl.Infof("deleting tunnel tunnel_id='%s' %s", params.TunnelId, principalLogFields(principal))

	_, tunnelLifecycle, err := s.lifecycleFactory(ctx)
	if err != nil {
		dl.Errorf("delete tunnel lifecycle initialization failed tunnel_id='%s' %s: %v", params.TunnelId, principalLogFields(principal), err)
		return &api.DeleteTunnelInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}

	disconnectedAt := time.Now().UTC()
	var tunnel *persistence.Tunnel
	if err := s.store.WithTx(ctx, func(tx persistence.Queryer) error {
		if err := lockTunnelScope(ctx, tx, params.TunnelId); err != nil {
			return err
		}
		var err error
		tunnel, err = s.requireManagedTunnelWithQuery(ctx, tx, principal, params.TunnelId)
		if err != nil {
			return err
		}
		attachments, err := s.store.TunnelAttachments.ListByTunnelCrossOrg(ctx, tx, tunnel.ID)
		if err != nil {
			return err
		}
		if err := deprovisionAttachmentPolicies(ctx, tunnelLifecycle, attachments); err != nil {
			dl.Errorf("delete tunnel attachment deprovision failed tunnel_id='%s' %s: %v", tunnel.ID, principalLogFields(principal), err)
			return err
		}
		if err := tunnelLifecycle.Deprovision(ctx, automation.DeprovisionTunnelSpec{
			ServiceID:                 optionalStringValue(tunnel.ZitiServiceID),
			BindPolicyID:              optionalStringValue(tunnel.BindPolicyID),
			ServiceEdgeRouterPolicyID: optionalStringValue(tunnel.ServiceEdgeRouterPolicyID),
		}); err != nil {
			dl.Errorf("delete tunnel deprovision failed tunnel_id='%s' %s: %v", tunnel.ID, principalLogFields(principal), err)
			return err
		}
		activeServe, err := s.store.TunnelServes.GetActiveByTunnel(ctx, tx, tunnel.ID, tunnel.OrganizationID)
		if err != nil && !errors.Is(err, persistence.ErrNotFound) {
			return err
		}
		if err == nil {
			if err := s.store.TunnelServes.UpdateState(ctx, tx, activeServe.ID, persistence.TunnelServeStateDisconnected, &disconnectedAt); err != nil {
				return err
			}
		}
		if err := s.store.TunnelServes.DeleteByTunnel(ctx, tx, tunnel.ID, tunnel.OrganizationID); err != nil {
			return err
		}
		if err := s.detachAndSoftDeleteAttachments(ctx, tx, attachments, persistence.TunnelAttachmentStateDisconnected, disconnectedAt); err != nil {
			return err
		}
		if err := s.store.TunnelGrants.DeleteByTunnelCrossOrg(ctx, tx, tunnel.ID); err != nil {
			return err
		}
		return s.store.Tunnels.Delete(ctx, tx, tunnel.ID, tunnel.OrganizationID)
	}); err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			return &api.DeleteTunnelNotFound{Code: "not_found", Message: "tunnel not found"}, nil
		}
		dl.Errorf("delete tunnel persistence failed tunnel_id='%s' %s: %v", params.TunnelId, principalLogFields(principal), err)
		return &api.DeleteTunnelInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}

	dl.Infof("deleted tunnel tunnel_id='%s' %s", tunnel.ID, principalLogFields(principal))
	return &api.DeleteTunnelNoContent{}, nil
}
