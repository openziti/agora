package controller

import (
	"context"
	"errors"

	"github.com/michaelquigley/df/dl"
	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/internal/persistence"
)

func (s *Service) GetWorkgroup(ctx context.Context, params api.GetWorkgroupParams) (api.GetWorkgroupRes, error) {
	principal, err := requireAccountPrincipal(ctx)
	if err != nil {
		return &api.GetWorkgroupUnauthorized{Code: "unauthorized", Message: "unauthorized"}, nil
	}

	wg, _, err := s.requireVisibleWorkgroup(ctx, principal, params.WorkgroupId)
	if err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			return &api.GetWorkgroupNotFound{Code: "not_found", Message: "workgroup not found"}, nil
		}
		dl.Errorf("get workgroup failed workgroup_id='%s' %s: %v", params.WorkgroupId, principalLogFields(principal), err)
		return &api.GetWorkgroupInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}

	participating, err := s.participatingOrgIDs(ctx, s.store.DB(), wg)
	if err != nil {
		return &api.GetWorkgroupInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}
	return mapWorkgroup(wg, participating), nil
}
