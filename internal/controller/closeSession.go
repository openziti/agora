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

func (s *Service) CloseSession(ctx context.Context, req api.OptCloseSessionRequest, params api.CloseSessionParams) (api.CloseSessionRes, error) {
	principal, err := requireAccountPrincipal(ctx)
	if err != nil {
		return &api.CloseSessionUnauthorized{Code: "unauthorized", Message: "unauthorized"}, nil
	}

	sess, err := s.store.Sessions.GetByID(ctx, s.store.DB(), params.SessionId)
	if err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			return &api.CloseSessionNotFound{Code: "not_found", Message: "session not found"}, nil
		}
		return &api.CloseSessionInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}
	isProvider := sess.ProviderAccountID == principal.AccountID
	isConsumer := sess.ConsumerAccountID == principal.AccountID
	if !isProvider && !isConsumer {
		return &api.CloseSessionNotFound{Code: "not_found", Message: "session not found"}, nil
	}

	reason := persistence.SessionCloseReasonProviderClose
	if isConsumer && !isProvider {
		reason = persistence.SessionCloseReasonConsumerClose
	}
	if req.Set && req.Value.Reason.Set {
		switch req.Value.Reason.Value {
		case api.SessionCloseReasonProviderClose:
			if !isProvider {
				return &api.CloseSessionForbidden{Code: "not_provider", Message: "only the provider may close with reason 'provider_close'"}, nil
			}
			reason = persistence.SessionCloseReasonProviderClose
		case api.SessionCloseReasonConsumerClose:
			if !isConsumer {
				return &api.CloseSessionForbidden{Code: "not_consumer", Message: "only the consumer may close with reason 'consumer_close'"}, nil
			}
			reason = persistence.SessionCloseReasonConsumerClose
		case api.SessionCloseReasonContractViolation:
			if !isProvider {
				return &api.CloseSessionForbidden{Code: "not_provider", Message: "only the provider may close with reason 'contract_violation'"}, nil
			}
			reason = persistence.SessionCloseReasonContractViolation
		default:
			return &api.CloseSessionBadRequest{Code: "invalid_request", Message: "reason must be provider_close, consumer_close, or contract_violation"}, nil
		}
	}
	detail := ""
	if req.Set && req.Value.Detail.Set {
		detail = req.Value.Detail.Value
	}

	if err := s.teardownSession(ctx, sess, reason, detail); err != nil {
		return &api.CloseSessionInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}
	dl.Infof("closed session id='%s' reason='%s' %s", sess.ID, reason, principalLogFields(principal))
	return &api.CloseSessionNoContent{}, nil
}

// teardownSession closes the session with the supplied reason and
// tears down any backing tunnel. Idempotent on already-closed sessions.
func (s *Service) teardownSession(ctx context.Context, sess *persistence.Session, reason persistence.SessionCloseReason, detail string) error {
	if sess.State == persistence.SessionStateClosed {
		return nil
	}
	disconnectedAt := time.Now().UTC()
	return s.store.WithTx(ctx, func(tx persistence.Queryer) error {
		var tunnel *persistence.Tunnel
		attachments := []persistence.TunnelAttachment{}
		if sess.TunnelID != nil {
			if err := lockTunnelScope(ctx, tx, *sess.TunnelID); err != nil {
				return err
			}
			var err error
			tunnel, err = s.store.Tunnels.GetByID(ctx, tx, *sess.TunnelID)
			if err != nil && !errors.Is(err, persistence.ErrNotFound) {
				return err
			}
			if tunnel != nil {
				attachments, err = s.store.TunnelAttachments.ListByTunnelCrossOrg(ctx, tx, tunnel.ID)
				if err != nil {
					return err
				}
				_, tunnelLifecycle, err := s.lifecycleFactory(ctx)
				if err != nil {
					return err
				}
				if err := deprovisionAttachmentPolicies(ctx, tunnelLifecycle, attachments); err != nil {
					return err
				}
				spec := automation.DeprovisionTunnelSpec{}
				if tunnel.ZitiServiceID != nil {
					spec.ServiceID = *tunnel.ZitiServiceID
				}
				if tunnel.BindPolicyID != nil {
					spec.BindPolicyID = *tunnel.BindPolicyID
				}
				if tunnel.ServiceEdgeRouterPolicyID != nil {
					spec.ServiceEdgeRouterPolicyID = *tunnel.ServiceEdgeRouterPolicyID
				}
				if err := tunnelLifecycle.Deprovision(ctx, spec); err != nil {
					return err
				}
			}
		}
		if tunnel != nil {
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
			if err := s.store.Tunnels.Delete(ctx, tx, tunnel.ID, tunnel.OrganizationID); err != nil {
				return err
			}
		}
		closed, err := s.store.Sessions.MarkClosed(ctx, tx, sess.ID, reason, detail)
		if err != nil {
			return err
		}
		return s.recordSessionClosed(ctx, tx, closed, reason, detail)
	})
}
