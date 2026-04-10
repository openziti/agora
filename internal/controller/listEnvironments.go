package controller

import (
	"context"

	"github.com/michaelquigley/df/dl"
	"github.com/openziti/agora/internal/api"
)

func (s *Service) ListEnvironments(ctx context.Context) (api.ListEnvironmentsRes, error) {
	principal, err := requireAccountPrincipal(ctx)
	if err != nil {
		dl.Warn("unauthorized list environments request")
		return &api.ListEnvironmentsUnauthorized{Code: "unauthorized", Message: "unauthorized"}, nil
	}
	dl.Debugf("listing environments %s", principalLogFields(principal))
	environments, err := s.store.Environments.ListByOrganization(ctx, s.store.DB(), principal.OrganizationID)
	if err != nil {
		dl.Errorf("list environments failed %s: %v", principalLogFields(principal), err)
		return &api.ListEnvironmentsInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}
	resp := make(api.ListEnvironmentsResponse, 0, len(environments))
	for i := range environments {
		resp = append(resp, *mapEnvironment(&environments[i]))
	}
	dl.Infof("listed %d environment(s) %s", len(resp), principalLogFields(principal))
	return &resp, nil
}
