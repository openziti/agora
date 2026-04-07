package controller

import (
	"context"
	"errors"

	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/internal/persistence"
)

func (s *Service) DeleteEnvironment(ctx context.Context, params api.DeleteEnvironmentParams) (api.DeleteEnvironmentRes, error) {
	principal, err := requireAccountPrincipal(ctx)
	if err != nil {
		return &api.DeleteEnvironmentUnauthorized{Code: "unauthorized", Message: "unauthorized"}, nil
	}
	if err := s.store.Environments.Delete(ctx, s.store.DB(), params.EnvironmentId, principal.OrganizationID); err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			return &api.DeleteEnvironmentNotFound{Code: "not_found", Message: "environment not found"}, nil
		}
		return &api.DeleteEnvironmentInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}
	return &api.DeleteEnvironmentNoContent{}, nil
}
