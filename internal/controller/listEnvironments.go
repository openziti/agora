package controller

import (
	"context"

	"github.com/openziti/agora/internal/api"
)

func (s *Service) ListEnvironments(ctx context.Context) (api.ListEnvironmentsRes, error) {
	principal, err := requireAccountPrincipal(ctx)
	if err != nil {
		return &api.ListEnvironmentsUnauthorized{Code: "unauthorized", Message: "unauthorized"}, nil
	}
	environments, err := s.store.Environments.ListByOrganization(ctx, s.store.DB(), principal.OrganizationID)
	if err != nil {
		return &api.ListEnvironmentsInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}
	resp := make(api.ListEnvironmentsResponse, 0, len(environments))
	for i := range environments {
		resp = append(resp, *mapEnvironment(&environments[i]))
	}
	return &resp, nil
}
