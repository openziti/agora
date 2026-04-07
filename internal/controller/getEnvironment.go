package controller

import (
	"context"
	"errors"

	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/internal/persistence"
)

func (s *Service) GetEnvironment(ctx context.Context, params api.GetEnvironmentParams) (api.GetEnvironmentRes, error) {
	principal, err := requireAccountPrincipal(ctx)
	if err != nil {
		return &api.GetEnvironmentUnauthorized{Code: "unauthorized", Message: "unauthorized"}, nil
	}
	env, err := s.store.Environments.GetByID(ctx, s.store.DB(), params.EnvironmentId)
	if err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			return &api.GetEnvironmentNotFound{Code: "not_found", Message: "environment not found"}, nil
		}
		return &api.GetEnvironmentInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}
	if env.OrganizationID != principal.OrganizationID {
		return &api.GetEnvironmentNotFound{Code: "not_found", Message: "environment not found"}, nil
	}
	return mapEnvironment(env), nil
}
