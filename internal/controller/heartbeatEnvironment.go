package controller

import (
	"context"
	"errors"
	"time"

	"github.com/michaelquigley/df/dl"
	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/internal/persistence"
)

func (s *Service) HeartbeatEnvironment(ctx context.Context, params api.HeartbeatEnvironmentParams) (api.HeartbeatEnvironmentRes, error) {
	principal, err := requireAccountPrincipal(ctx)
	if err != nil {
		dl.Warnf("unauthorized environment heartbeat environment_id='%s'", params.EnvironmentId)
		return &api.HeartbeatEnvironmentUnauthorized{Code: "unauthorized", Message: "unauthorized"}, nil
	}

	env, err := s.requireOwnedEnvironment(ctx, principal, params.EnvironmentId)
	if err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			dl.Debugf("environment heartbeat not found environment_id='%s' %s", params.EnvironmentId, principalLogFields(principal))
			return &api.HeartbeatEnvironmentNotFound{Code: "not_found", Message: "environment not found"}, nil
		}
		dl.Errorf("environment heartbeat lookup failed environment_id='%s' %s: %v", params.EnvironmentId, principalLogFields(principal), err)
		return &api.HeartbeatEnvironmentInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}

	if err := s.store.WithTx(ctx, func(tx persistence.Queryer) error {
		if err := s.store.Environments.UpdateLastSeen(ctx, tx, params.EnvironmentId, time.Now().UTC()); err != nil {
			return err
		}
		return s.recordEnvironmentHeartbeat(ctx, tx, env, 0)
	}); err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			dl.Debugf("environment heartbeat update not found environment_id='%s' %s", params.EnvironmentId, principalLogFields(principal))
			return &api.HeartbeatEnvironmentNotFound{Code: "not_found", Message: "environment not found"}, nil
		}
		dl.Errorf("environment heartbeat update failed environment_id='%s' %s: %v", params.EnvironmentId, principalLogFields(principal), err)
		return &api.HeartbeatEnvironmentInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}

	return &api.HeartbeatEnvironmentNoContent{}, nil
}
