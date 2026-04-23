package controller

import (
	"context"
	"errors"

	"github.com/michaelquigley/df/dl"
	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/internal/persistence"
)

func (s *Service) ChangeWorkgroupMembershipRole(ctx context.Context, req *api.ChangeWorkgroupMembershipRoleRequest, params api.ChangeWorkgroupMembershipRoleParams) (api.ChangeWorkgroupMembershipRoleRes, error) {
	principal, err := requireAccountPrincipal(ctx)
	if err != nil {
		return &api.ChangeWorkgroupMembershipRoleUnauthorized{Code: "unauthorized", Message: "unauthorized"}, nil
	}

	wg, _, err := s.requireWorkgroupAdmin(ctx, principal, params.WorkgroupId)
	if err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			return &api.ChangeWorkgroupMembershipRoleNotFound{Code: "not_found", Message: "workgroup not found"}, nil
		}
		if errors.Is(err, errForbidden) {
			return &api.ChangeWorkgroupMembershipRoleForbidden{Code: "insufficient_role", Message: "workgroup admin role required"}, nil
		}
		return &api.ChangeWorkgroupMembershipRoleInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}

	target, err := s.store.WorkgroupMemberships.GetByID(ctx, s.store.DB(), params.MembershipId)
	if err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			return &api.ChangeWorkgroupMembershipRoleNotFound{Code: "not_found", Message: "membership not found"}, nil
		}
		return &api.ChangeWorkgroupMembershipRoleInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}
	if target.WorkgroupID != wg.ID {
		return &api.ChangeWorkgroupMembershipRoleNotFound{Code: "not_found", Message: "membership not found"}, nil
	}
	if target.OrganizationID != principal.OrganizationID {
		return &api.ChangeWorkgroupMembershipRoleForbidden{Code: "cross_org_membership", Message: "cannot manage memberships in another organization"}, nil
	}

	newRole := persistence.WorkgroupMembershipRole(req.Role)
	if newRole == target.Role {
		return mapWorkgroupMembership(target, ""), nil
	}

	if err := s.store.WithTx(ctx, func(tx persistence.Queryer) error {
		if err := s.store.WorkgroupMemberships.UpdateRole(ctx, tx, target.ID, newRole); err != nil {
			return err
		}
		count, err := s.store.WorkgroupMemberships.CountAdmins(ctx, tx, wg.ID)
		if err != nil {
			return err
		}
		if count == 0 {
			return errLastAdmin
		}
		return nil
	}); err != nil {
		if errors.Is(err, errLastAdmin) {
			return &api.ChangeWorkgroupMembershipRoleConflict{Code: "last_admin", Message: "workgroup must retain at least one admin"}, nil
		}
		dl.Errorf("change workgroup membership role failed workgroup_id='%s' membership_id='%s' %s: %v", wg.ID, target.ID, principalLogFields(principal), err)
		return &api.ChangeWorkgroupMembershipRoleInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}

	updated, err := s.store.WorkgroupMemberships.GetByID(ctx, s.store.DB(), target.ID)
	if err != nil {
		return &api.ChangeWorkgroupMembershipRoleInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}
	dl.Infof("changed workgroup membership role workgroup_id='%s' membership_id='%s' role='%s' %s", wg.ID, updated.ID, updated.Role, principalLogFields(principal))
	return mapWorkgroupMembership(updated, ""), nil
}

var errLastAdmin = errors.New("last admin")
