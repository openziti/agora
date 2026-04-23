package controller

import (
	"context"
	"errors"

	"github.com/michaelquigley/df/dl"
	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/internal/persistence"
)

func (s *Service) DeleteWorkgroup(ctx context.Context, params api.DeleteWorkgroupParams) (api.DeleteWorkgroupRes, error) {
	principal, err := requireAccountPrincipal(ctx)
	if err != nil {
		return &api.DeleteWorkgroupUnauthorized{Code: "unauthorized", Message: "unauthorized"}, nil
	}

	wg, _, err := s.requireWorkgroupAdmin(ctx, principal, params.WorkgroupId)
	if err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			return &api.DeleteWorkgroupNotFound{Code: "not_found", Message: "workgroup not found"}, nil
		}
		if errors.Is(err, errForbidden) {
			return &api.DeleteWorkgroupForbidden{Code: "insufficient_role", Message: "workgroup admin role required"}, nil
		}
		dl.Errorf("delete workgroup auth failed workgroup_id='%s' %s: %v", params.WorkgroupId, principalLogFields(principal), err)
		return &api.DeleteWorkgroupInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}

	if err := s.store.WithTx(ctx, func(tx persistence.Queryer) error {
		if err := s.store.WorkgroupMemberships.SoftDeleteAllForWorkgroup(ctx, tx, wg.ID); err != nil {
			return err
		}
		return s.store.Workgroups.MarkDeleted(ctx, tx, wg.ID)
	}); err != nil {
		dl.Errorf("delete workgroup persistence failed workgroup_id='%s' %s: %v", wg.ID, principalLogFields(principal), err)
		return &api.DeleteWorkgroupInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}

	dl.Infof("deleted workgroup workgroup_id='%s' %s", wg.ID, principalLogFields(principal))
	return &api.DeleteWorkgroupNoContent{}, nil
}
