package controller

import (
	"context"

	"github.com/michaelquigley/df/dl"
	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/internal/persistence"
)

func (s *Service) GetDashboardSummary(ctx context.Context) (api.GetDashboardSummaryRes, error) {
	principal, err := requireAccountPrincipal(ctx)
	if err != nil {
		return &api.GetDashboardSummaryUnauthorized{Code: "unauthorized", Message: "unauthorized"}, nil
	}

	org, err := s.store.Organizations.GetByID(ctx, s.store.DB(), principal.OrganizationID)
	if err != nil {
		dl.Errorf("get dashboard summary organization lookup failed %s: %v", principalLogFields(principal), err)
		return &api.GetDashboardSummaryInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}

	counts, err := persistence.DashboardCounts(ctx, s.store.DB(), principal.OrganizationID, principal.AccountID)
	if err != nil {
		dl.Errorf("get dashboard summary aggregation failed %s: %v", principalLogFields(principal), err)
		return &api.GetDashboardSummaryInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}

	return &api.DashboardSummaryResponse{
		Account: api.DashboardAccount{
			AccountId:        principal.AccountID,
			Email:            principal.Email,
			OrganizationId:   principal.OrganizationID,
			OrganizationName: org.Name,
			Role:             api.DashboardAccountRole(principal.Role),
		},
		Stats:  mapDashboardStats(counts.Stats),
		Ribbon: mapDashboardRibbon(counts.Ribbon),
	}, nil
}
