package controller

import (
	"context"

	"github.com/michaelquigley/df/dl"
	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/internal/persistence"
)

func (s *Service) GetWorkgroupsActivity(ctx context.Context, params api.GetWorkgroupsActivityParams) (api.GetWorkgroupsActivityRes, error) {
	principal, err := requireAccountPrincipal(ctx)
	if err != nil {
		return &api.GetWorkgroupsActivityUnauthorized{Code: "unauthorized", Message: "unauthorized"}, nil
	}

	window, err := dashboardWindowDuration(params.Window.Or(api.DashboardWindow24h))
	if err != nil {
		return &api.GetWorkgroupsActivityBadRequest{Code: "invalid_request", Message: err.Error()}, nil
	}

	byWorkgroup, err := persistence.EnvelopesByVisibleWorkgroups(ctx, s.store.DB(), principal.OrganizationID, principal.AccountID, window)
	if err != nil {
		dl.Errorf("get workgroups activity failed %s: %v", principalLogFields(principal), err)
		return &api.GetWorkgroupsActivityInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}

	return &api.WorkgroupsActivityResponse{ByWorkgroup: mapDashboardWorkgroupActivity(byWorkgroup)}, nil
}
