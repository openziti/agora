package controller

import (
	"context"

	"github.com/michaelquigley/df/dl"
	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/internal/persistence"
)

func (s *Service) GetDashboardEnvironments(ctx context.Context) (api.GetDashboardEnvironmentsRes, error) {
	principal, err := requireAccountPrincipal(ctx)
	if err != nil {
		return &api.GetDashboardEnvironmentsUnauthorized{Code: "unauthorized", Message: "unauthorized"}, nil
	}

	environments, err := persistence.EnvironmentStatuses(ctx, s.store.DB(), principal.OrganizationID)
	if err != nil {
		dl.Errorf("get dashboard environments failed %s: %v", principalLogFields(principal), err)
		return &api.GetDashboardEnvironmentsInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}

	resp := mapDashboardEnvironments(environments)
	return &resp, nil
}
