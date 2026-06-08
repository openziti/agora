package controller

import (
	"context"
	"errors"
	"time"

	"github.com/michaelquigley/df/dl"
	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/internal/persistence"
)

func (s *Service) RemoveTunnelGrant(ctx context.Context, params api.RemoveTunnelGrantParams) (api.RemoveTunnelGrantRes, error) {
	principal, err := requireAccountPrincipal(ctx)
	if err != nil {
		dl.Warnf("unauthorized remove tunnel grant request tunnel_id='%s' account_id='%s'", params.TunnelId, params.AccountId)
		return &api.RemoveTunnelGrantUnauthorized{Code: "unauthorized", Message: "unauthorized"}, nil
	}
	dl.Infof("removing tunnel grant tunnel_id='%s' account_id='%s' %s", params.TunnelId, params.AccountId, principalLogFields(principal))

	_, tunnelLifecycle, err := s.lifecycleFactory(ctx)
	if err != nil {
		return &api.RemoveTunnelGrantInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}

	disconnectedAt := time.Now().UTC()
	if err := s.store.WithTx(ctx, func(tx persistence.Queryer) error {
		if err := lockTunnelScope(ctx, tx, params.TunnelId); err != nil {
			return err
		}
		tunnel, err := s.requireManagedTunnelWithQuery(ctx, tx, principal, params.TunnelId)
		if err != nil {
			return err
		}
		granted, err := s.store.TunnelGrants.IsGranted(ctx, tx, tunnel.ID, params.AccountId)
		if err != nil {
			return err
		}
		if !granted {
			return persistence.ErrNotFound
		}
		attachments, err := s.store.TunnelAttachments.ListByTunnelAndAccountCrossOrg(ctx, tx, tunnel.ID, params.AccountId)
		if err != nil {
			return err
		}
		if err := deprovisionAttachmentPolicies(ctx, tunnelLifecycle, attachments); err != nil {
			dl.Errorf("remove tunnel grant attachment deprovision failed tunnel_id='%s' account_id='%s' %s: %v", tunnel.ID, params.AccountId, principalLogFields(principal), err)
			return err
		}
		if err := s.detachAndSoftDeleteAttachments(ctx, tx, attachments, persistence.TunnelAttachmentStateDisconnected, disconnectedAt); err != nil {
			return err
		}
		return s.store.TunnelGrants.DeleteCrossOrg(ctx, tx, params.TunnelId, params.AccountId)
	}); err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			return &api.RemoveTunnelGrantNotFound{Code: "not_found", Message: "grant not found"}, nil
		}
		return &api.RemoveTunnelGrantInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}

	return &api.RemoveTunnelGrantNoContent{}, nil
}
