package controller

import (
	"context"
	"errors"

	"github.com/michaelquigley/df/dl"
	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/internal/persistence"
)

func (s *Service) RemoveWorkgroupMember(ctx context.Context, params api.RemoveWorkgroupMemberParams) (api.RemoveWorkgroupMemberRes, error) {
	principal, err := requireAccountPrincipal(ctx)
	if err != nil {
		return &api.RemoveWorkgroupMemberUnauthorized{Code: "unauthorized", Message: "unauthorized"}, nil
	}

	wg, callerMembership, err := s.requireVisibleWorkgroup(ctx, principal, params.WorkgroupId)
	if err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			return &api.RemoveWorkgroupMemberNotFound{Code: "not_found", Message: "workgroup not found"}, nil
		}
		return &api.RemoveWorkgroupMemberInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}

	target, err := s.store.WorkgroupMemberships.GetByID(ctx, s.store.DB(), params.MembershipId)
	if err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			return &api.RemoveWorkgroupMemberNotFound{Code: "not_found", Message: "membership not found"}, nil
		}
		return &api.RemoveWorkgroupMemberInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}
	if target.WorkgroupID != wg.ID {
		return &api.RemoveWorkgroupMemberNotFound{Code: "not_found", Message: "membership not found"}, nil
	}

	isSelf := target.AccountID == principal.AccountID
	isAdmin := callerMembership.Role == persistence.WorkgroupMembershipRoleAdmin
	if !isSelf && !isAdmin {
		return &api.RemoveWorkgroupMemberForbidden{Code: "insufficient_role", Message: "workgroup admin role required to remove other members"}, nil
	}
	if isAdmin && !isSelf && target.OrganizationID != principal.OrganizationID {
		return &api.RemoveWorkgroupMemberForbidden{Code: "cross_org_membership", Message: "cannot remove members from another organization"}, nil
	}

	if err := s.store.WithTx(ctx, func(tx persistence.Queryer) error {
		if err := s.store.WorkgroupMemberships.MarkDeleted(ctx, tx, target.ID); err != nil {
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
			return &api.RemoveWorkgroupMemberConflict{Code: "last_admin", Message: "workgroup must retain at least one admin"}, nil
		}
		dl.Errorf("remove workgroup member persistence failed workgroup_id='%s' membership_id='%s' %s: %v", wg.ID, target.ID, principalLogFields(principal), err)
		return &api.RemoveWorkgroupMemberInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}

	dl.Infof("removed workgroup member workgroup_id='%s' membership_id='%s' account_id='%s' %s", wg.ID, target.ID, target.AccountID, principalLogFields(principal))
	return &api.RemoveWorkgroupMemberNoContent{}, nil
}
