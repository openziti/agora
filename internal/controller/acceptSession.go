package controller

import (
	"context"
	"errors"
	"fmt"

	"github.com/michaelquigley/df/dl"
	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/internal/fabric/openziti/automation"
	"github.com/openziti/agora/internal/persistence"
)

func (s *Service) AcceptSession(ctx context.Context, req *api.AcceptSessionRequest, params api.AcceptSessionParams) (api.AcceptSessionRes, error) {
	principal, err := requireAccountPrincipal(ctx)
	if err != nil {
		return &api.AcceptSessionUnauthorized{Code: "unauthorized", Message: "unauthorized"}, nil
	}

	sess, err := s.store.Sessions.GetByID(ctx, s.store.DB(), params.SessionId)
	if err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			return &api.AcceptSessionNotFound{Code: "not_found", Message: "session not found"}, nil
		}
		return &api.AcceptSessionInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}
	if sess.ProviderAccountID != principal.AccountID {
		// non-provider: indistinguishable from a non-visible session for the 404-leak rule,
		// but we return 403 here because the provider alone is the authorized accept caller
		// and the spec prescribes not_provider for this case.
		if sess.ConsumerAccountID != principal.AccountID {
			return &api.AcceptSessionNotFound{Code: "not_found", Message: "session not found"}, nil
		}
		return &api.AcceptSessionForbidden{Code: "not_provider", Message: "only the advertisement owner may accept this session"}, nil
	}
	if sess.State != persistence.SessionStateProposed {
		return &api.AcceptSessionConflict{Code: "invalid_state", Message: fmt.Sprintf("session is in state '%s'", sess.State)}, nil
	}

	env, err := s.requireOwnedEnvironment(ctx, principal, req.EnvironmentId)
	if err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			return &api.AcceptSessionBadRequest{Code: "unknown_environment", Message: "environmentId is not enrolled to the provider account"}, nil
		}
		return &api.AcceptSessionInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}

	// Evaluate contract admission if the advertisement references one.
	var snapshotJSON []byte
	ad, err := s.store.Advertisements.GetByID(ctx, s.store.DB(), sess.AdvertisementID)
	if err != nil {
		return &api.AcceptSessionInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}
	if ad.ContractID != nil {
		contract, err := s.store.Contracts.GetByID(ctx, s.store.DB(), *ad.ContractID)
		if err != nil {
			return &api.AcceptSessionInternalServerError{Code: "internal_error", Message: err.Error()}, nil
		}
		consumer, err := s.store.Accounts.GetByID(ctx, s.store.DB(), sess.ConsumerAccountID)
		if err != nil {
			return &api.AcceptSessionInternalServerError{Code: "internal_error", Message: err.Error()}, nil
		}
		if err := s.evaluateContractAdmission(ctx, consumer, contract); err != nil {
			detail := err.Error()
			if closeErr := s.store.WithTx(ctx, func(tx persistence.Queryer) error {
				_, err := s.markSessionClosedWithAudit(ctx, tx, sess.ID, persistence.SessionCloseReasonContractViolation, detail)
				return err
			}); closeErr != nil {
				return &api.AcceptSessionInternalServerError{Code: "internal_error", Message: closeErr.Error()}, nil
			}
			dl.Warnf("contract admission failed session_id='%s' contract_id='%s' reason='%v' %s", sess.ID, contract.ID, err, principalLogFields(principal))
			switch {
			case errors.Is(err, errContractViolationMemb):
				return &api.AcceptSessionForbidden{Code: "contract_violation_memberships", Message: "consumer does not hold required workgroup memberships"}, nil
			case errors.Is(err, errContractViolationMat):
				return &api.AcceptSessionForbidden{Code: "contract_violation_maturity", Message: "consumer account does not meet the minimum account age"}, nil
			default:
				return &api.AcceptSessionInternalServerError{Code: "internal_error", Message: err.Error()}, nil
			}
		}
		snapshotJSON, err = snapshotContract(contract)
		if err != nil {
			return &api.AcceptSessionInternalServerError{Code: "internal_error", Message: err.Error()}, nil
		}
	}

	if _, err := s.store.Sessions.MarkAccepting(ctx, s.store.DB(), sess.ID); err != nil {
		return &api.AcceptSessionInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}

	tunnelID := persistence.NewResourceID(persistence.PrefixTunnel)
	serviceName := tunnelServiceName(tunnelID)
	tunnelName := fmt.Sprintf("session-%s", sess.ID)

	_, tunnelLifecycle, err := s.lifecycleFactory(ctx)
	if err != nil {
		detail := err.Error()
		if closeErr := s.store.WithTx(ctx, func(tx persistence.Queryer) error {
			_, markErr := s.markSessionClosedWithAudit(ctx, tx, sess.ID, persistence.SessionCloseReasonTunnelFailed, detail)
			return markErr
		}); closeErr != nil {
			return &api.AcceptSessionInternalServerError{Code: "internal_error", Message: closeErr.Error()}, nil
		}
		return &api.AcceptSessionInternalServerError{Code: "tunnel_provisioning_failed", Message: detail}, nil
	}

	provisioned, err := tunnelLifecycle.Provision(ctx, automation.TunnelSpec{
		OrganizationID:        principal.OrganizationID,
		AccountID:             principal.AccountID,
		EnvironmentID:         env.ID,
		TunnelID:              tunnelID,
		TunnelName:            tunnelName,
		ServiceName:           serviceName,
		EnvironmentIdentityID: env.ZitiIdentityID,
		Version:               automation.DefaultAgoraVersion,
	})
	if err != nil {
		if closeErr := s.store.WithTx(ctx, func(tx persistence.Queryer) error {
			_, markErr := s.markSessionClosedWithAudit(ctx, tx, sess.ID, persistence.SessionCloseReasonTunnelFailed, err.Error())
			return markErr
		}); closeErr != nil {
			return &api.AcceptSessionInternalServerError{Code: "internal_error", Message: closeErr.Error()}, nil
		}
		return &api.AcceptSessionInternalServerError{Code: "tunnel_provisioning_failed", Message: err.Error()}, nil
	}

	backendTarget := fmt.Sprintf("session:%s", sess.ID)
	if req.BackendAddress.Set && req.BackendAddress.Value != "" {
		backendTarget = req.BackendAddress.Value
	}
	tunnel := persistence.Tunnel{
		ID:                        tunnelID,
		OrganizationID:            principal.OrganizationID,
		AccountID:                 principal.AccountID,
		EnvironmentID:             env.ID,
		Name:                      tunnelName,
		Mode:                      sess.TunnelMode,
		BackendTarget:             stringPtr(backendTarget),
		ZitiServiceID:             &provisioned.ServiceID,
		BindPolicyID:              &provisioned.BindPolicyID,
		ServiceEdgeRouterPolicyID: &provisioned.ServiceEdgeRouterPolicyID,
		State:                     persistence.TunnelStateActive,
	}

	var activeSess *persistence.Session
	if err := s.store.WithTx(ctx, func(tx persistence.Queryer) error {
		if _, err := s.store.Tunnels.Create(ctx, tx, tunnel); err != nil {
			return err
		}
		if sess.ProviderAccountID != sess.ConsumerAccountID {
			if _, err := s.store.TunnelGrants.Create(ctx, tx, persistence.TunnelAccountGrant{
				TunnelID:       tunnel.ID,
				AccountID:      sess.ConsumerAccountID,
				OrganizationID: sess.ConsumerOrganizationID,
			}); err != nil {
				return err
			}
		}
		if len(snapshotJSON) > 0 {
			if err := s.store.Sessions.WriteContractSnapshot(ctx, tx, sess.ID, snapshotJSON); err != nil {
				return err
			}
		}
		out, err := s.store.Sessions.MarkActive(ctx, tx, sess.ID, tunnel.ID)
		if err != nil {
			return err
		}
		activeSess = out
		return s.recordSessionAccepted(ctx, tx, activeSess, ad.ContractID)
	}); err != nil {
		_ = tunnelLifecycle.Deprovision(ctx, automation.DeprovisionTunnelSpec{
			ServiceID:                 provisioned.ServiceID,
			BindPolicyID:              provisioned.BindPolicyID,
			ServiceEdgeRouterPolicyID: provisioned.ServiceEdgeRouterPolicyID,
		})
		if closeErr := s.store.WithTx(ctx, func(tx persistence.Queryer) error {
			_, markErr := s.markSessionClosedWithAudit(ctx, tx, sess.ID, persistence.SessionCloseReasonTunnelFailed, err.Error())
			return markErr
		}); closeErr != nil {
			return &api.AcceptSessionInternalServerError{Code: "internal_error", Message: closeErr.Error()}, nil
		}
		dl.Errorf("accept session persistence failed session_id='%s' %s: %v", sess.ID, principalLogFields(principal), err)
		return &api.AcceptSessionInternalServerError{Code: "tunnel_provisioning_failed", Message: err.Error()}, nil
	}

	dl.Infof("accepted session id='%s' tunnel_id='%s' environment_id='%s' %s",
		activeSess.ID, tunnel.ID, env.ID, principalLogFields(principal))
	activeWithDisplay, err := s.store.Sessions.GetByIDWithDisplay(ctx, s.store.DB(), activeSess.ID)
	if err != nil {
		dl.Errorf("accept session display lookup failed session_id='%s' %s: %v", activeSess.ID, principalLogFields(principal), err)
		return &api.AcceptSessionInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}
	return mapSession(activeWithDisplay, principal.OrganizationID), nil
}
