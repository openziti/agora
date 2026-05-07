package controller

import (
	"context"

	"github.com/michaelquigley/df/dl"
	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/internal/persistence"
)

func (s *Service) Whoami(ctx context.Context) (api.WhoamiRes, error) {
	principal, err := requireAccountPrincipal(ctx)
	if err != nil {
		return &api.WhoamiUnauthorized{Code: "unauthorized", Message: "unauthorized"}, nil
	}

	org, err := s.store.Organizations.GetByID(ctx, s.store.DB(), principal.OrganizationID)
	if err != nil {
		dl.Errorf("whoami organization lookup failed %s: %v", principalLogFields(principal), err)
		return &api.WhoamiInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}

	account := mapDashboardAccount(principal, org)
	return &account, nil
}

func mapDashboardAccount(principal *accountPrincipal, org *persistence.Organization) api.DashboardAccount {
	return api.DashboardAccount{
		AccountId:        principal.AccountID,
		Email:            principal.Email,
		OrganizationId:   principal.OrganizationID,
		OrganizationName: org.Name,
		Role:             api.DashboardAccountRole(principal.Role),
	}
}
