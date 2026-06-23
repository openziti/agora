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

func (s *Service) DisableEnvironment(ctx context.Context, params api.DisableEnvironmentParams) (api.DisableEnvironmentRes, error) {
	principal, err := requireAccountPrincipal(ctx)
	if err != nil {
		dl.Warnf("unauthorized disable environment request environment_id='%s'", params.EnvironmentId)
		return &api.DisableEnvironmentUnauthorized{Code: "unauthorized", Message: "unauthorized"}, nil
	}
	logFields := principalLogFields(principal)
	dl.Infof("disabling environment environment_id='%s' %s", params.EnvironmentId, logFields)

	env, err := s.store.Environments.GetByID(ctx, s.store.DB(), params.EnvironmentId)
	if err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			dl.Warnf("disable environment not found environment_id='%s' %s", params.EnvironmentId, logFields)
			return &api.DisableEnvironmentNotFound{Code: "not_found", Message: "environment not found"}, nil
		}
		dl.Errorf("disable environment lookup failed environment_id='%s' %s: %v", params.EnvironmentId, logFields, err)
		return &api.DisableEnvironmentInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}
	if env.OrganizationID != principal.OrganizationID || env.AccountID != principal.AccountID {
		dl.Warnf("disable environment rejected for non-owned environment environment_id='%s' %s", params.EnvironmentId, logFields)
		return &api.DisableEnvironmentNotFound{Code: "not_found", Message: "environment not found"}, nil
	}

	if err := s.disableEnvironmentInternal(ctx, env, logFields); err != nil {
		dl.Errorf("disable environment cleanup failed environment_id='%s' %s: %v", env.ID, logFields, err)
		return &api.DisableEnvironmentInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}
	dl.Infof("disabled environment environment_id='%s' %s", env.ID, logFields)

	return &api.DisableEnvironmentNoContent{}, nil
}

func (s *Service) disableEnvironmentInternal(ctx context.Context, env *persistence.Environment, logFields string) error {
	return s.store.WithTx(ctx, func(tx persistence.Queryer) error {
		return s.disableEnvironmentWithLocks(ctx, tx, env.ID, env.OrganizationID, env.AccountID, logFields)
	})
}

func (s *Service) disableEnvironmentWithLocks(ctx context.Context, tx persistence.Queryer, environmentID, organizationID, accountID, logFields string) error {
	if err := lockEnvironmentScope(ctx, tx, environmentID); err != nil {
		return err
	}
	env, err := s.store.Environments.GetByID(ctx, tx, environmentID)
	if err != nil {
		return err
	}
	if env.OrganizationID != organizationID || env.AccountID != accountID {
		return persistence.ErrNotFound
	}

	tunnels, err := s.store.Tunnels.ListByEnvironment(ctx, tx, env.ID, env.OrganizationID)
	if err != nil {
		return err
	}
	dl.Infof("disable environment found %d tunnel(s) for environment_id='%s' %s", len(tunnels), env.ID, logFields)

	consumerAttachments, err := s.store.TunnelAttachments.ListConsumerAttachmentsWithPolicy(ctx, tx, env.ID)
	if err != nil {
		return err
	}
	tunnelIDs := make([]string, 0, len(tunnels)+len(consumerAttachments))
	for i := range tunnels {
		tunnelIDs = append(tunnelIDs, tunnels[i].ID)
	}
	for i := range consumerAttachments {
		tunnelIDs = append(tunnelIDs, consumerAttachments[i].TunnelID)
	}
	if err := lockTunnelScopes(ctx, tx, tunnelIDs); err != nil {
		return err
	}

	tunnels, err = s.store.Tunnels.ListByEnvironment(ctx, tx, env.ID, env.OrganizationID)
	if err != nil {
		return err
	}
	consumerAttachments, err = s.store.TunnelAttachments.ListConsumerAttachmentsWithPolicy(ctx, tx, env.ID)
	if err != nil {
		return err
	}
	ownedTunnelAttachments := []persistence.TunnelAttachment{}
	for i := range tunnels {
		attachments, err := s.store.TunnelAttachments.ListByTunnelCrossOrg(ctx, tx, tunnels[i].ID)
		if err != nil {
			return err
		}
		ownedTunnelAttachments = append(ownedTunnelAttachments, attachments...)
	}
	attachments := uniqueTunnelAttachments(ownedTunnelAttachments, consumerAttachments)

	envLifecycle, tunnelLifecycle, err := s.lifecycleFactory(ctx)
	if err != nil {
		return err
	}

	if err := deprovisionAttachmentPolicies(ctx, tunnelLifecycle, attachments); err != nil {
		return err
	}

	for i := range tunnels {
		tunnel := tunnels[i]
		dl.Infof("deprovisioning tunnel tunnel_id='%s' environment_id='%s' %s", tunnel.ID, env.ID, logFields)
		if err := tunnelLifecycle.Deprovision(ctx, automation.DeprovisionTunnelSpec{
			ServiceID:                 optionalStringValue(tunnel.ZitiServiceID),
			BindPolicyID:              optionalStringValue(tunnel.BindPolicyID),
			ServiceEdgeRouterPolicyID: optionalStringValue(tunnel.ServiceEdgeRouterPolicyID),
		}); err != nil {
			return err
		}
	}

	// explicitly evict the retiring environment's fabric terminators before its identity is deleted. the
	// environment may have been hosting account-owned standalone tunnels -- which are NOT in `tunnels`
	// (that lists only this environment's session tunnels) and survive retirement -- so their host
	// terminators are keyed by this environment's identity and would otherwise be stranded. this is
	// especially necessary for direct tunnels, which keep no serve row the DB could enumerate them from
	// (finding C2).
	if err := tunnelLifecycle.EvictTerminatorsByIdentity(ctx, env.ZitiIdentityID); err != nil {
		return err
	}

	disableSpec := automation.DeprovisionEnvironmentSpec{IdentityID: env.ZitiIdentityID}
	if env.EdgeRouterPolicyID != nil {
		disableSpec.EdgeRouterPolicyID = *env.EdgeRouterPolicyID
	}
	if err := envLifecycle.Disable(ctx, disableSpec); err != nil {
		return err
	}

	disconnectedAt := time.Now().UTC()
	if err := s.detachAndSoftDeleteAttachments(ctx, tx, attachments, persistence.TunnelAttachmentStateDisconnected, disconnectedAt); err != nil {
		return err
	}
	for i := range tunnels {
		activeServe, err := s.store.TunnelServes.GetActiveByTunnel(ctx, tx, tunnels[i].ID, env.OrganizationID)
		if err != nil && !errors.Is(err, persistence.ErrNotFound) {
			return err
		}
		if err == nil {
			if err := s.store.TunnelServes.UpdateState(ctx, tx, activeServe.ID, persistence.TunnelServeStateDisconnected, &disconnectedAt); err != nil {
				return err
			}
		}
		if err := s.store.TunnelServes.DeleteByTunnel(ctx, tx, tunnels[i].ID, env.OrganizationID); err != nil {
			return err
		}
		if err := s.store.TunnelGrants.DeleteByTunnelCrossOrg(ctx, tx, tunnels[i].ID); err != nil {
			return err
		}
		if err := s.store.Tunnels.Delete(ctx, tx, tunnels[i].ID, env.OrganizationID); err != nil {
			return err
		}
	}
	// tear down the environment's serve records on the account-owned standalone tunnels it was hosting.
	// those tunnels are not in `tunnels` (ListByEnvironment returns only session tunnels) and survive
	// retirement, and the tunnel_serves env-FK cascade does not fire because Environments.Delete is a
	// soft delete -- so without this an `active` serve record would linger and block re-serve from
	// another environment until the reaper stales it.
	if err := s.store.TunnelServes.DisconnectAndDeleteByEnvironment(ctx, tx, env.ID, env.OrganizationID, disconnectedAt); err != nil {
		return err
	}
	return s.store.Environments.Delete(ctx, tx, env.ID, env.OrganizationID)
}
