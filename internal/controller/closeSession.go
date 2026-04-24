package controller

import (
	"context"
	"errors"

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
	detail := ""
	if req.Set && req.Value.Reason.Set {
		detail = req.Value.Reason.Value
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
	if _, err := s.store.Sessions.MarkClosed(ctx, s.store.DB(), sess.ID, reason, detail); err != nil {
		return err
	}
	if sess.TunnelID == nil {
		return nil
	}
	tunnel, err := s.store.Tunnels.GetByID(ctx, s.store.DB(), *sess.TunnelID)
	if err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			return nil
		}
		return err
	}
	_, tunnelLifecycle, err := s.lifecycleFactory(ctx)
	if err != nil {
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
	if err := s.store.Tunnels.Delete(ctx, s.store.DB(), tunnel.ID, tunnel.OrganizationID); err != nil {
		return err
	}
	return nil
}
