package controller

import (
	"context"
	"errors"

	"github.com/michaelquigley/df/dl"
	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/internal/persistence"
)

func (s *Service) GetEnvironment(ctx context.Context, params api.GetEnvironmentParams) (api.GetEnvironmentRes, error) {
	principal, err := requireAccountPrincipal(ctx)
	if err != nil {
		dl.Warnf("unauthorized get environment request environment_id='%s'", params.EnvironmentId)
		return &api.GetEnvironmentUnauthorized{Code: "unauthorized", Message: "unauthorized"}, nil
	}
	dl.Debugf("getting environment environment_id='%s' %s", params.EnvironmentId, principalLogFields(principal))
	env, err := s.store.Environments.GetByID(ctx, s.store.DB(), params.EnvironmentId)
	if err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			dl.Warnf("get environment not found environment_id='%s' %s", params.EnvironmentId, principalLogFields(principal))
			return &api.GetEnvironmentNotFound{Code: "not_found", Message: "environment not found"}, nil
		}
		dl.Errorf("get environment failed environment_id='%s' %s: %v", params.EnvironmentId, principalLogFields(principal), err)
		return &api.GetEnvironmentInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}
	if env.OrganizationID != principal.OrganizationID {
		dl.Warnf("get environment rejected for non-owned environment environment_id='%s' %s", params.EnvironmentId, principalLogFields(principal))
		return &api.GetEnvironmentNotFound{Code: "not_found", Message: "environment not found"}, nil
	}
	dl.Debugf("got environment environment_id='%s' %s", params.EnvironmentId, principalLogFields(principal))
	return mapEnvironment(env), nil
}
