package controller

import (
	"context"
	"errors"

	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/internal/openziti/automation"
	"github.com/openziti/agora/internal/persistence"
)

func (s *Service) DisableEnvironment(ctx context.Context, params api.DisableEnvironmentParams) (api.DisableEnvironmentRes, error) {
	principal, err := requireAccountPrincipal(ctx)
	if err != nil {
		return &api.DisableEnvironmentUnauthorized{Code: "unauthorized", Message: "unauthorized"}, nil
	}

	env, err := s.store.Environments.GetByID(ctx, s.store.DB(), params.EnvironmentId)
	if err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			return &api.DisableEnvironmentNotFound{Code: "not_found", Message: "environment not found"}, nil
		}
		return &api.DisableEnvironmentInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}
	if env.OrganizationID != principal.OrganizationID || env.AccountID != principal.AccountID {
		return &api.DisableEnvironmentNotFound{Code: "not_found", Message: "environment not found"}, nil
	}

	tunnels, err := s.store.Tunnels.ListByEnvironment(ctx, s.store.DB(), env.ID, env.OrganizationID)
	if err != nil {
		return &api.DisableEnvironmentInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}

	envLifecycle, tunnelLifecycle, err := s.lifecycleFactory(ctx)
	if err != nil {
		return &api.DisableEnvironmentInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}

	for i := range tunnels {
		tunnel := tunnels[i]
		if tunnel.ZitiServiceID != nil {
			if err := tunnelLifecycle.Deprovision(ctx, automation.DeprovisionTunnelSpec{ServiceID: *tunnel.ZitiServiceID}); err != nil {
				return &api.DisableEnvironmentInternalServerError{Code: "internal_error", Message: err.Error()}, nil
			}
		}
	}

	disableSpec := automation.DeprovisionEnvironmentSpec{IdentityID: env.ZitiIdentityID}
	if env.EdgeRouterPolicyID != nil {
		disableSpec.EdgeRouterPolicyID = *env.EdgeRouterPolicyID
	}
	if err := envLifecycle.Disable(ctx, disableSpec); err != nil {
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
		return &api.DisableEnvironmentInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}

	return &api.DisableEnvironmentNoContent{}, nil
}
