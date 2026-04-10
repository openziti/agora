package controller

import (
	"context"
	"errors"

	"github.com/michaelquigley/df/dl"
	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/internal/openziti/automation"
	"github.com/openziti/agora/internal/persistence"
)

func (s *Service) DisableEnvironment(ctx context.Context, params api.DisableEnvironmentParams) (api.DisableEnvironmentRes, error) {
	principal, err := requireAccountPrincipal(ctx)
	if err != nil {
		dl.Warnf("unauthorized disable environment request environment_id='%s'", params.EnvironmentId)
		return &api.DisableEnvironmentUnauthorized{Code: "unauthorized", Message: "unauthorized"}, nil
	}
	dl.Infof("disabling environment environment_id='%s' %s", params.EnvironmentId, principalLogFields(principal))

	env, err := s.store.Environments.GetByID(ctx, s.store.DB(), params.EnvironmentId)
	if err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			dl.Warnf("disable environment not found environment_id='%s' %s", params.EnvironmentId, principalLogFields(principal))
			return &api.DisableEnvironmentNotFound{Code: "not_found", Message: "environment not found"}, nil
		}
		dl.Errorf("disable environment lookup failed environment_id='%s' %s: %v", params.EnvironmentId, principalLogFields(principal), err)
		return &api.DisableEnvironmentInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}
	if env.OrganizationID != principal.OrganizationID || env.AccountID != principal.AccountID {
		dl.Warnf("disable environment rejected for non-owned environment environment_id='%s' %s", params.EnvironmentId, principalLogFields(principal))
		return &api.DisableEnvironmentNotFound{Code: "not_found", Message: "environment not found"}, nil
	}

	tunnels, err := s.store.Tunnels.ListByEnvironment(ctx, s.store.DB(), env.ID, env.OrganizationID)
	if err != nil {
		dl.Errorf("disable environment tunnel lookup failed environment_id='%s' %s: %v", env.ID, principalLogFields(principal), err)
		return &api.DisableEnvironmentInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}
	dl.Infof("disable environment found %d tunnel(s) for environment_id='%s' %s", len(tunnels), env.ID, principalLogFields(principal))

	envLifecycle, tunnelLifecycle, err := s.lifecycleFactory(ctx)
	if err != nil {
		dl.Errorf("disable environment lifecycle initialization failed environment_id='%s' %s: %v", env.ID, principalLogFields(principal), err)
		return &api.DisableEnvironmentInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}

	for i := range tunnels {
		tunnel := tunnels[i]
		if tunnel.ZitiServiceID != nil {
			dl.Infof("deprovisioning tunnel service tunnel_id='%s' ziti_service_id='%s' environment_id='%s' %s", tunnel.ID, *tunnel.ZitiServiceID, env.ID, principalLogFields(principal))
			if err := tunnelLifecycle.Deprovision(ctx, automation.DeprovisionTunnelSpec{ServiceID: *tunnel.ZitiServiceID}); err != nil {
				dl.Errorf("disable environment tunnel deprovision failed tunnel_id='%s' environment_id='%s' %s: %v", tunnel.ID, env.ID, principalLogFields(principal), err)
				return &api.DisableEnvironmentInternalServerError{Code: "internal_error", Message: err.Error()}, nil
			}
		}
	}

	disableSpec := automation.DeprovisionEnvironmentSpec{IdentityID: env.ZitiIdentityID}
	if env.EdgeRouterPolicyID != nil {
		disableSpec.EdgeRouterPolicyID = *env.EdgeRouterPolicyID
	}
	if err := envLifecycle.Disable(ctx, disableSpec); err != nil {
		dl.Errorf("disable environment deprovision failed environment_id='%s' ziti_identity_id='%s' %s: %v", env.ID, env.ZitiIdentityID, principalLogFields(principal), err)
		return &api.DisableEnvironmentInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}

	if err := s.store.WithTx(ctx, func(tx persistence.Queryer) error {
		for i := range tunnels {
			if err := s.store.Tunnels.Delete(ctx, tx, tunnels[i].ID, env.OrganizationID); err != nil {
				return err
			}
		}
		return s.store.Environments.Delete(ctx, tx, env.ID, env.OrganizationID)
	}); err != nil {
		dl.Errorf("disable environment persistence cleanup failed environment_id='%s' %s: %v", env.ID, principalLogFields(principal), err)
		return &api.DisableEnvironmentInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}
	dl.Infof("disabled environment environment_id='%s' %s", env.ID, principalLogFields(principal))

	return &api.DisableEnvironmentNoContent{}, nil
}
