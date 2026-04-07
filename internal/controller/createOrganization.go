package controller

import (
	"context"

	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/internal/persistence"
)

func (s *Service) CreateOrganization(ctx context.Context, req *api.CreateOrganizationRequest) (api.CreateOrganizationRes, error) {
	if _, err := requireAdminPrincipal(ctx); err != nil {
		return &api.CreateOrganizationUnauthorized{Code: "unauthorized", Message: "unauthorized"}, nil
	}
	org, err := s.store.Organizations.Create(ctx, s.store.DB(), persistence.Organization{Name: req.Name})
	if err != nil {
		return &api.CreateOrganizationConflict{Code: "conflict", Message: err.Error()}, nil
	}
	return mapOrganization(org), nil
}
