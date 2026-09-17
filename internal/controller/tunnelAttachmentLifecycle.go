package controller

import (
	"context"
	"errors"
	"fmt"
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

	attachment, err := s.store.TunnelAttachments.GetByIDForAccount(ctx, s.store.DB(), params.AttachmentId, principal.OrganizationID, principal.AccountID)
	if err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			return &api.HeartbeatTunnelAttachmentNotFound{Code: "not_found", Message: "attachment not found"}, nil
		}
		return &api.HeartbeatTunnelAttachmentInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}

	heartbeatAt := time.Now().UTC()
	if attachmentNeedsDialPolicyReconciliation(attachment) {
		if _, err := s.reconcileTunnelAttachmentHeartbeat(ctx, principal, attachment, heartbeatAt); err != nil {
			if errors.Is(err, persistence.ErrNotFound) {
				return &api.HeartbeatTunnelAttachmentNotFound{Code: "not_found", Message: "attachment not found"}, nil
			}
			dl.Errorf("tunnel attachment heartbeat reconciliation failed attachment_id='%s' tunnel_id='%s' error='%v'", attachment.ID, attachment.TunnelID, err)
			return &api.HeartbeatTunnelAttachmentInternalServerError{Code: "internal_error", Message: err.Error()}, nil
		}
		return &api.HeartbeatTunnelAttachmentNoContent{}, nil
	}

	if err := s.store.TunnelAttachments.Heartbeat(ctx, s.store.DB(), params.AttachmentId, principal.OrganizationID, principal.AccountID, heartbeatAt); err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			return &api.HeartbeatTunnelAttachmentNotFound{Code: "not_found", Message: "attachment not found"}, nil
		}
		return &api.HeartbeatTunnelAttachmentInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}

	return &api.HeartbeatTunnelAttachmentNoContent{}, nil
}

func attachmentNeedsDialPolicyReconciliation(attachment *persistence.TunnelAttachment) bool {
	if attachment.Kind != persistence.TunnelAttachmentKindProxy || attachment.State == persistence.TunnelAttachmentStateDisconnected {
		return false
	}
	return attachment.State == persistence.TunnelAttachmentStateStale || attachment.DisconnectedAt != nil
}

func (s *Service) reconcileTunnelAttachmentHeartbeat(ctx context.Context, principal *accountPrincipal, attachment *persistence.TunnelAttachment, heartbeatAt time.Time) (bool, error) {
	_, tunnelLifecycle, err := s.lifecycleFactory(ctx)
	if err != nil {
		return false, err
	}

	policyCreated := false
	dialPolicyID := ""
	err = s.store.WithTx(ctx, func(tx persistence.Queryer) error {
		if err := lockEnvironmentScope(ctx, tx, attachment.EnvironmentID); err != nil {
			return err
		}
		if err := lockTunnelScope(ctx, tx, attachment.TunnelID); err != nil {
			return err
		}

		current, err := s.store.TunnelAttachments.GetByIDForAccount(ctx, tx, attachment.ID, principal.OrganizationID, principal.AccountID)
		if err != nil {
			return err
		}
		if current.State == persistence.TunnelAttachmentStateDisconnected {
			return persistence.ErrNotFound
		}
		if !attachmentNeedsDialPolicyReconciliation(current) {
			return s.store.TunnelAttachments.Heartbeat(ctx, tx, current.ID, principal.OrganizationID, principal.AccountID, heartbeatAt)
		}

		environment, err := s.store.Environments.GetByID(ctx, tx, current.EnvironmentID)
		if err != nil {
			return err
		}
		tunnel, err := s.store.Tunnels.GetByID(ctx, tx, current.TunnelID)
		if err != nil {
			return err
		}
		if environment.OrganizationID != current.OrganizationID || environment.AccountID != current.AccountID {
			return persistence.ErrNotFound
		}
		allowed, err := s.canConnectToTunnel(ctx, tx, tunnel, environment, principal)
		if err != nil {
			return err
		}
		if !allowed {
			return persistence.ErrNotFound
		}
		if tunnel.ZitiServiceID == nil {
			return fmt.Errorf("tunnel '%s' is missing service metadata", tunnel.ID)
		}

		dialPolicyID, policyCreated, err = tunnelLifecycle.EnsureAttachmentDialPolicy(ctx, automation.TunnelAccessSpec{
			OrganizationID:        current.OrganizationID,
			AccountID:             current.AccountID,
			EnvironmentID:         current.EnvironmentID,
			TunnelID:              tunnel.ID,
			TunnelName:            tunnel.Name,
			AttachmentID:          current.ID,
			EnvironmentIdentityID: environment.ZitiIdentityID,
			ServiceID:             *tunnel.ZitiServiceID,
			Version:               automation.DefaultAgoraVersion,
		})
		if err != nil {
			return err
		}
		if err := s.store.TunnelAttachments.HeartbeatWithDialPolicy(ctx, tx, current.ID, principal.OrganizationID, principal.AccountID, dialPolicyID, heartbeatAt); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		if policyCreated && dialPolicyID != "" {
			if compensationErr := s.compensateCreatedAttachmentDialPolicy(ctx, tunnelLifecycle, attachment, dialPolicyID); compensationErr != nil {
				return false, errors.Join(err, compensationErr)
			}
		}
		return false, err
	}

	if dialPolicyID != "" {
		dl.Infof(
			"reconciled tunnel attachment heartbeat attachment_id='%s' tunnel_id='%s' dial_policy_id='%s' policy_created=%t",
			attachment.ID,
			attachment.TunnelID,
			dialPolicyID,
			policyCreated,
		)
	}
	return policyCreated, nil
}

func (s *Service) compensateCreatedAttachmentDialPolicy(ctx context.Context, tunnelLifecycle tunnelLifecycle, attachment *persistence.TunnelAttachment, dialPolicyID string) error {
	compensationCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()

	return s.store.WithTx(compensationCtx, func(tx persistence.Queryer) error {
		if err := lockEnvironmentScope(compensationCtx, tx, attachment.EnvironmentID); err != nil {
			return err
		}
		if err := lockTunnelScope(compensationCtx, tx, attachment.TunnelID); err != nil {
			return err
		}

		current, err := s.store.TunnelAttachments.GetByID(compensationCtx, tx, attachment.ID)
		if err == nil && current.DialPolicyID != nil && *current.DialPolicyID == dialPolicyID {
			return nil
		}
		if err != nil && !errors.Is(err, persistence.ErrNotFound) {
			return err
		}
		if err := tunnelLifecycle.Deprovision(compensationCtx, automation.DeprovisionTunnelSpec{DialPolicyID: dialPolicyID}); err != nil {
			return fmt.Errorf("compensate attachment dial policy '%s': %w", dialPolicyID, err)
		}
		return nil
	})
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
	if attachment.Kind == persistence.TunnelAttachmentKindDialer {
		deleted, err := s.detachDialerAttachmentByID(ctx, tunnelLifecycle, principal, attachment.ID)
		if err != nil {
			if errors.Is(err, persistence.ErrNotFound) {
				return &api.DeleteTunnelAttachmentNotFound{Code: "not_found", Message: "attachment not found"}, nil
			}
			return &api.DeleteTunnelAttachmentInternalServerError{Code: "internal_error", Message: err.Error()}, nil
		}
		dl.Infof("deleted dialer attachment attachment_id='%s' tunnel_id='%s' %s", deleted.ID, deleted.TunnelID, principalLogFields(principal))
		return &api.DeleteTunnelAttachmentNoContent{}, nil
	}

	deleted, err := s.detachProxyAttachmentByID(ctx, tunnelLifecycle, principal, attachment.ID)
	if err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			return &api.DeleteTunnelAttachmentNotFound{Code: "not_found", Message: "attachment not found"}, nil
		}
		return &api.DeleteTunnelAttachmentInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}

	dl.Infof("deleted tunnel attachment attachment_id='%s' tunnel_id='%s' %s", deleted.ID, deleted.TunnelID, principalLogFields(principal))
	return &api.DeleteTunnelAttachmentNoContent{}, nil
}

func (s *Service) detachProxyAttachmentByID(ctx context.Context, tunnelLifecycle tunnelLifecycle, principal *accountPrincipal, attachmentID string) (*persistence.TunnelAttachment, error) {
	disconnectedAt := time.Now().UTC()
	var attachment *persistence.TunnelAttachment
	err := s.store.WithTx(ctx, func(tx persistence.Queryer) error {
		current, err := s.store.TunnelAttachments.GetByIDForAccount(ctx, tx, attachmentID, principal.OrganizationID, principal.AccountID)
		if err != nil {
			return err
		}
		if current.Kind != persistence.TunnelAttachmentKindProxy || current.State == persistence.TunnelAttachmentStateDisconnected {
			return persistence.ErrNotFound
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
		if current.Kind != persistence.TunnelAttachmentKindProxy || current.State == persistence.TunnelAttachmentStateDisconnected {
			return persistence.ErrNotFound
		}
		attachment = current
		if err := deprovisionAttachmentPolicies(ctx, tunnelLifecycle, []persistence.TunnelAttachment{*attachment}); err != nil {
			return err
		}
		return s.detachTunnel(ctx, tx, *attachment, persistence.TunnelAttachmentStateDisconnected, &disconnectedAt)
	})
	return attachment, err
}
