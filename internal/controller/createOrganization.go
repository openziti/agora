package controller

import (
	"context"

	"github.com/michaelquigley/df/dl"
	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/internal/persistence"
)

func (s *Service) CreateOrganization(ctx context.Context, req *api.CreateOrganizationRequest) (api.CreateOrganizationRes, error) {
	if _, err := requireAdminPrincipal(ctx); err != nil {
		dl.Warn("unauthorized create organization request")
		return &api.CreateOrganizationUnauthorized{Code: "unauthorized", Message: "unauthorized"}, nil
	}
	dl.Infof("creating organization name='%s' %s", req.Name, adminLogFields())
	org, err := s.store.Organizations.Create(ctx, s.store.DB(), persistence.Organization{Name: req.Name})
	if err != nil {
		dl.Errorf("create organization failed name='%s': %v", req.Name, err)
		return &api.CreateOrganizationConflict{Code: "conflict", Message: err.Error()}, nil
	}
	dl.Infof("created organization id='%s' name='%s'", org.ID, org.Name)
	return mapOrganization(org), nil
}
