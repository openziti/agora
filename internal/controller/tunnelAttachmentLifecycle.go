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

func (s *Service) HeartbeatTunnelAttachment(ctx context.Context, params api.HeartbeatTunnelAttachmentParams) (api.HeartbeatTunnelAttachmentRes, error) {
	principal, err := requireAccountPrincipal(ctx)
	if err != nil {
		dl.Warnf("unauthorized tunnel attachment heartbeat attachment_id='%s'", params.AttachmentId)
		return &api.HeartbeatTunnelAttachmentUnauthorized{Code: "unauthorized", Message: "unauthorized"}, nil
	}

	if _, err := s.store.TunnelAttachments.GetByIDForAccount(ctx, s.store.DB(), params.AttachmentId, principal.OrganizationID, principal.AccountID); err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			return &api.HeartbeatTunnelAttachmentNotFound{Code: "not_found", Message: "attachment not found"}, nil
		}
		return &api.HeartbeatTunnelAttachmentInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}
	if err := s.store.TunnelAttachments.Heartbeat(ctx, s.store.DB(), params.AttachmentId, principal.OrganizationID, principal.AccountID, time.Now().UTC()); err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			return &api.HeartbeatTunnelAttachmentNotFound{Code: "not_found", Message: "attachment not found"}, nil
		}
		return &api.HeartbeatTunnelAttachmentInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}

	return &api.HeartbeatTunnelAttachmentNoContent{}, nil
}

func (s *Service) DeleteTunnelAttachment(ctx context.Context, params api.DeleteTunnelAttachmentParams) (api.DeleteTunnelAttachmentRes, error) {
	principal, err := requireAccountPrincipal(ctx)
	if err != nil {
		dl.Warnf("unauthorized delete tunnel attachment request attachment_id='%s'", params.AttachmentId)
		return &api.DeleteTunnelAttachmentUnauthorized{Code: "unauthorized", Message: "unauthorized"}, nil
	}

	attachment, err := s.store.TunnelAttachments.GetByIDForAccount(ctx, s.store.DB(), params.AttachmentId, principal.OrganizationID, principal.AccountID)
	if err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			return &api.DeleteTunnelAttachmentNotFound{Code: "not_found", Message: "attachment not found"}, nil
		}
		return &api.DeleteTunnelAttachmentInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}

	_, tunnelLifecycle, err := s.lifecycleFactory(ctx)
	if err != nil {
		return &api.DeleteTunnelAttachmentInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}
	if attachment.DialPolicyID != nil {
		if err := tunnelLifecycle.Deprovision(ctx, automation.DeprovisionTunnelSpec{DialPolicyID: *attachment.DialPolicyID}); err != nil {
			return &api.DeleteTunnelAttachmentInternalServerError{Code: "internal_error", Message: err.Error()}, nil
		}
	}

	disconnectedAt := time.Now().UTC()
	if err := s.store.WithTx(ctx, func(tx persistence.Queryer) error {
		return s.detachTunnel(ctx, tx, *attachment, persistence.TunnelAttachmentStateDisconnected, &disconnectedAt)
	}); err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			return &api.DeleteTunnelAttachmentNotFound{Code: "not_found", Message: "attachment not found"}, nil
		}
		return &api.DeleteTunnelAttachmentInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}

	dl.Infof("deleted tunnel attachment attachment_id='%s' tunnel_id='%s' %s", attachment.ID, attachment.TunnelID, principalLogFields(principal))
	return &api.DeleteTunnelAttachmentNoContent{}, nil
}
