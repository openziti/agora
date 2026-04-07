package controller

import (
	"context"

	"github.com/openziti/agora/internal/api"
)

func (s *Service) ListOrganizations(ctx context.Context) (api.ListOrganizationsRes, error) {
	if _, err := requireAdminPrincipal(ctx); err != nil {
		return &api.ListOrganizationsUnauthorized{Code: "unauthorized", Message: "unauthorized"}, nil
	}
	organizations, err := s.store.Organizations.List(ctx, s.store.DB())
	if err != nil {
		return &api.ListOrganizationsInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}
	resp := make(api.ListOrganizationsResponse, 0, len(organizations))
	for i := range organizations {
		resp = append(resp, *mapOrganization(&organizations[i]))
	}
	return &resp, nil
}
