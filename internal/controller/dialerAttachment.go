package controller

import (
	"context"
	"errors"
	"time"

	"github.com/michaelquigley/df/dl"
	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/internal/persistence"
)

func (s *Service) GetActiveDialerAttachment(ctx context.Context, params api.GetActiveDialerAttachmentParams) (api.GetActiveDialerAttachmentRes, error) {
	principal, err := requireAccountPrincipal(ctx)
	if err != nil {
		dl.Warnf("unauthorized get active dialer attachment request environment_id='%s' tunnel='%s'", params.EnvironmentId, params.Tunnel)
		return &api.GetActiveDialerAttachmentUnauthorized{Code: "unauthorized", Message: "unauthorized"}, nil
	}

	tunnel, attachment, resErr := s.resolveActiveDialerAttachment(ctx, principal, params.EnvironmentId, params.Tunnel)
	if resErr != nil {
		switch {
		case errors.Is(resErr, errTunnelReferenceAmbiguous):
			return &api.GetActiveDialerAttachmentConflict{Code: "conflict", Message: "tunnel name is ambiguous"}, nil
		case errors.Is(resErr, errEnvironmentForbidden):
			return &api.GetActiveDialerAttachmentForbidden{Code: "forbidden", Message: "environment forbidden"}, nil
		case errors.Is(resErr, persistence.ErrNotFound):
			return &api.GetActiveDialerAttachmentNotFound{Code: "not_found", Message: "dialer attachment not found"}, nil
		default:
			return &api.GetActiveDialerAttachmentInternalServerError{Code: "internal_error", Message: resErr.Error()}, nil
		}
	}

	return mapActiveDialerAttachment(tunnel, attachment), nil
}

func (s *Service) DetachDialerAttachment(ctx context.Context, params api.DetachDialerAttachmentParams) (api.DetachDialerAttachmentRes, error) {
	principal, err := requireAccountPrincipal(ctx)
	if err != nil {
		dl.Warnf("unauthorized detach dialer attachment request environment_id='%s' tunnel='%s'", params.EnvironmentId, params.Tunnel)
		return &api.DetachDialerAttachmentUnauthorized{Code: "unauthorized", Message: "unauthorized"}, nil
	}

	_, tunnelLifecycle, err := s.lifecycleFactory(ctx)
	if err != nil {
		return &api.DetachDialerAttachmentInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}
	tunnel, attachment, resErr := s.detachActiveDialerAttachment(ctx, tunnelLifecycle, principal, params.EnvironmentId, params.Tunnel)
	if resErr != nil {
		switch {
		case errors.Is(resErr, errTunnelReferenceAmbiguous):
			return &api.DetachDialerAttachmentConflict{Code: "conflict", Message: "tunnel name is ambiguous"}, nil
		case errors.Is(resErr, errEnvironmentForbidden):
			return &api.DetachDialerAttachmentForbidden{Code: "forbidden", Message: "environment forbidden"}, nil
		case errors.Is(resErr, persistence.ErrNotFound):
			return &api.DetachDialerAttachmentNotFound{Code: "not_found", Message: "dialer attachment not found"}, nil
		default:
			return &api.DetachDialerAttachmentInternalServerError{Code: "internal_error", Message: resErr.Error()}, nil
		}
	}

	dl.Infof("detached dialer attachment attachment_id='%s' tunnel_id='%s' %s", attachment.ID, tunnel.ID, principalLogFields(principal))
	return &api.DetachDialerAttachmentNoContent{}, nil
}

var (
	errEnvironmentForbidden      = errors.New("environment forbidden")
	errTunnelAttachmentNotDialer = errors.New("tunnel attachment is not a dialer")
)

func (s *Service) resolveActiveDialerAttachment(ctx context.Context, principal *accountPrincipal, environmentID, tunnelRef string) (*persistence.Tunnel, *persistence.TunnelAttachment, error) {
	env, err := s.requireOwnedEnvironment(ctx, principal, environmentID)
	if err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			return nil, nil, errEnvironmentForbidden
		}
		return nil, nil, err
	}

	tunnel, err := s.resolveConsumerTunnel(ctx, s.store.DB(), principal, env, tunnelRef)
	if err != nil {
		return nil, nil, err
	}

	attachment, err := s.store.TunnelAttachments.GetActiveDialerByEnvironmentAndTunnel(ctx, s.store.DB(), env.ID, tunnel.ID)
	if err != nil {
		return nil, nil, err
	}
	return tunnel, attachment, nil
}

func (s *Service) detachActiveDialerAttachment(ctx context.Context, tunnelLifecycle tunnelLifecycle, principal *accountPrincipal, environmentID, tunnelRef string) (*persistence.Tunnel, *persistence.TunnelAttachment, error) {
	disconnectedAt := time.Now().UTC()
	var tunnel *persistence.Tunnel
	var attachment *persistence.TunnelAttachment
	err := s.store.WithTx(ctx, func(tx persistence.Queryer) error {
		if err := lockEnvironmentScope(ctx, tx, environmentID); err != nil {
			return err
		}

		env, err := s.requireOwnedEnvironmentWithQuery(ctx, tx, principal, environmentID)
		if err != nil {
			if errors.Is(err, persistence.ErrNotFound) {
				return errEnvironmentForbidden
			}
			return err
		}
		tunnel, err = s.resolveConsumerTunnel(ctx, tx, principal, env, tunnelRef)
		if err != nil {
			return err
		}
		if err := lockTunnelScope(ctx, tx, tunnel.ID); err != nil {
			return err
		}
		tunnel, err = s.store.Tunnels.GetByID(ctx, tx, tunnel.ID)
		if err != nil {
			return err
		}
		allowed, err := s.canConnectToTunnel(ctx, tx, tunnel, env, principal)
		if err != nil {
			return err
		}
		if !allowed {
			return persistence.ErrNotFound
		}
		attachment, err = s.store.TunnelAttachments.GetActiveDialerByEnvironmentAndTunnel(ctx, tx, env.ID, tunnel.ID)
		if err != nil {
			return err
		}
		if attachment.OrganizationID != principal.OrganizationID || attachment.AccountID != principal.AccountID {
			return persistence.ErrNotFound
		}
		if err := deprovisionAttachmentPolicies(ctx, tunnelLifecycle, []persistence.TunnelAttachment{*attachment}); err != nil {
			return err
		}
		if err := s.detachTunnel(ctx, tx, *attachment, persistence.TunnelAttachmentStateDisconnected, &disconnectedAt); err != nil {
			return err
		}
		return s.store.TunnelAttachments.SoftDeleteByID(ctx, tx, attachment.ID)
	})
	return tunnel, attachment, err
}

func (s *Service) detachDialerAttachmentByID(ctx context.Context, tunnelLifecycle tunnelLifecycle, principal *accountPrincipal, attachmentID string) (*persistence.TunnelAttachment, error) {
	disconnectedAt := time.Now().UTC()
	var attachment *persistence.TunnelAttachment
	err := s.store.WithTx(ctx, func(tx persistence.Queryer) error {
		current, err := s.store.TunnelAttachments.GetByIDForAccount(ctx, tx, attachmentID, principal.OrganizationID, principal.AccountID)
		if err != nil {
			return err
		}
		if current.Kind != persistence.TunnelAttachmentKindDialer {
			return errTunnelAttachmentNotDialer
		}
		if err := lockEnvironmentScope(ctx, tx, current.EnvironmentID); err != nil {
			return err
		}
		if err := lockTunnelScope(ctx, tx, current.TunnelID); err != nil {
			return err
		}
		current, err = s.store.TunnelAttachments.GetByIDForAccount(ctx, tx, attachmentID, principal.OrganizationID, principal.AccountID)
		if err != nil {
			return err
		}
		if current.Kind != persistence.TunnelAttachmentKindDialer {
			return errTunnelAttachmentNotDialer
		}
		attachment = current
		if err := deprovisionAttachmentPolicies(ctx, tunnelLifecycle, []persistence.TunnelAttachment{*attachment}); err != nil {
			return err
		}
		if err := s.detachTunnel(ctx, tx, *attachment, persistence.TunnelAttachmentStateDisconnected, &disconnectedAt); err != nil {
			return err
		}
		return s.store.TunnelAttachments.SoftDeleteByID(ctx, tx, attachment.ID)
	})
	return attachment, err
}

func mapActiveDialerAttachment(tunnel *persistence.Tunnel, attachment *persistence.TunnelAttachment) *api.ActiveDialerAttachment {
	return &api.ActiveDialerAttachment{
		Attachment: *mapTunnelAttachment(attachment),
		TunnelId:   tunnel.ID,
		TunnelMode: api.TunnelMode(tunnel.Mode),
	}
}
