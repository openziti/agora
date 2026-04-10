package controller

import (
	"context"

	"github.com/michaelquigley/df/dl"
	"github.com/openziti/agora/internal/api"
)

func (s *Service) ListOrganizations(ctx context.Context) (api.ListOrganizationsRes, error) {
	if _, err := requireAdminPrincipal(ctx); err != nil {
		dl.Warn("unauthorized list organizations request")
		return &api.ListOrganizationsUnauthorized{Code: "unauthorized", Message: "unauthorized"}, nil
	}
	dl.Debugf("listing organizations %s", adminLogFields())
	organizations, err := s.store.Organizations.List(ctx, s.store.DB())
	if err != nil {
		dl.Errorf("list organizations failed: %v", err)
		return &api.ListOrganizationsInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}
	resp := make(api.ListOrganizationsResponse, 0, len(organizations))
	for i := range organizations {
		resp = append(resp, *mapOrganization(&organizations[i]))
	}
	dl.Infof("listed %d organization(s)", len(resp))
	return &resp, nil
}
