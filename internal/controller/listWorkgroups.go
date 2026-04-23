package controller

import (
	"context"

	"github.com/michaelquigley/df/dl"
	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/internal/persistence"
)

func (s *Service) ListWorkgroups(ctx context.Context) (api.ListWorkgroupsRes, error) {
	principal, err := requireAccountPrincipal(ctx)
	if err != nil {
		return &api.ListWorkgroupsUnauthorized{Code: "unauthorized", Message: "unauthorized"}, nil
	}

	workgroups, err := s.store.Workgroups.ListAccessibleByAccount(ctx, s.store.DB(), principal.AccountID)
	if err != nil {
		dl.Errorf("list workgroups failed %s: %v", principalLogFields(principal), err)
		return &api.ListWorkgroupsInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}

	result := make(api.ListWorkgroupsResponse, 0, len(workgroups))
	for i := range workgroups {
		wg := workgroups[i]
		participating, err := s.participatingOrgIDs(ctx, s.store.DB(), &wg)
		if err != nil {
			dl.Errorf("list workgroups participating-orgs failed workgroup_id='%s' %s: %v", wg.ID, principalLogFields(principal), err)
			return &api.ListWorkgroupsInternalServerError{Code: "internal_error", Message: err.Error()}, nil
		}
		result = append(result, *mapWorkgroup(&wg, participating))
	}
	_ = persistence.WorkgroupStateActive // keep import if unused elsewhere
	return &result, nil
}
