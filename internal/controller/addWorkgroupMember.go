package controller

import (
	"context"
	"errors"
	"strings"

	"github.com/michaelquigley/df/dl"
	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/internal/persistence"
)

func (s *Service) AddWorkgroupMember(ctx context.Context, req *api.AddWorkgroupMemberRequest, params api.AddWorkgroupMemberParams) (api.AddWorkgroupMemberRes, error) {
	principal, err := requireAccountPrincipal(ctx)
	if err != nil {
		return &api.AddWorkgroupMemberUnauthorized{Code: "unauthorized", Message: "unauthorized"}, nil
	}

	wg, _, err := s.requireWorkgroupAdmin(ctx, principal, params.WorkgroupId)
	if err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			return &api.AddWorkgroupMemberNotFound{Code: "not_found", Message: "workgroup not found"}, nil
		}
		if errors.Is(err, errForbidden) {
			return &api.AddWorkgroupMemberForbidden{Code: "insufficient_role", Message: "workgroup admin role required"}, nil
		}
		dl.Errorf("add workgroup member auth failed workgroup_id='%s' %s: %v", params.WorkgroupId, principalLogFields(principal), err)
		return &api.AddWorkgroupMemberInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}

	email := strings.ToLower(strings.TrimSpace(req.AccountEmail))
	if email == "" {
		return &api.AddWorkgroupMemberBadRequest{Code: "invalid_request", Message: "accountEmail is required"}, nil
	}
	target, err := s.store.Accounts.FindByEmail(ctx, s.store.DB(), email)
	if err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			return &api.AddWorkgroupMemberBadRequest{Code: "unknown_reference", Message: "no account with that email in your organization"}, nil
		}
		return &api.AddWorkgroupMemberInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}
	if target.OrganizationID != principal.OrganizationID {
		return &api.AddWorkgroupMemberForbidden{Code: "cross_org_membership", Message: "cannot add accounts from another organization"}, nil
	}

	role := persistence.WorkgroupMembershipRoleMember
	if req.Role.Set {
		role = persistence.WorkgroupMembershipRole(req.Role.Value)
	}

	existing, err := s.store.WorkgroupMemberships.GetByWorkgroupAndAccount(ctx, s.store.DB(), wg.ID, target.ID)
	if err != nil && !errors.Is(err, persistence.ErrNotFound) {
		return &api.AddWorkgroupMemberInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}
	if existing != nil {
		return &api.AddWorkgroupMemberConflict{Code: "already_a_member", Message: "account is already a member of this workgroup"}, nil
	}

	created, err := s.store.WorkgroupMemberships.Create(ctx, s.store.DB(), persistence.WorkgroupMembership{
		WorkgroupID:    wg.ID,
		OrganizationID: target.OrganizationID,
		AccountID:      target.ID,
		Role:           role,
	})
	if err != nil {
		dl.Errorf("add workgroup member persistence failed workgroup_id='%s' account_id='%s' %s: %v", wg.ID, target.ID, principalLogFields(principal), err)
		return &api.AddWorkgroupMemberInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}
	dl.Infof("added workgroup member workgroup_id='%s' membership_id='%s' account_id='%s' role='%s' %s", wg.ID, created.ID, target.ID, created.Role, principalLogFields(principal))
	return mapWorkgroupMembership(created, target.Email), nil
}
