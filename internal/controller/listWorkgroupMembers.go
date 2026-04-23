package controller

import (
	"context"
	"errors"

	"github.com/michaelquigley/df/dl"
	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/internal/persistence"
)

func (s *Service) ListWorkgroupMembers(ctx context.Context, params api.ListWorkgroupMembersParams) (api.ListWorkgroupMembersRes, error) {
	principal, err := requireAccountPrincipal(ctx)
	if err != nil {
		return &api.ListWorkgroupMembersUnauthorized{Code: "unauthorized", Message: "unauthorized"}, nil
	}

	if _, _, err := s.requireVisibleWorkgroup(ctx, principal, params.WorkgroupId); err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			return &api.ListWorkgroupMembersNotFound{Code: "not_found", Message: "workgroup not found"}, nil
		}
		dl.Errorf("list workgroup members auth failed workgroup_id='%s' %s: %v", params.WorkgroupId, principalLogFields(principal), err)
		return &api.ListWorkgroupMembersInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}

	memberships, err := s.store.WorkgroupMemberships.ListByWorkgroup(ctx, s.store.DB(), params.WorkgroupId)
	if err != nil {
		dl.Errorf("list workgroup members failed workgroup_id='%s' %s: %v", params.WorkgroupId, principalLogFields(principal), err)
		return &api.ListWorkgroupMembersInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}

	sameOrgIDs := make([]string, 0, len(memberships))
	for _, m := range memberships {
		if m.OrganizationID == principal.OrganizationID {
			sameOrgIDs = append(sameOrgIDs, m.AccountID)
		}
	}
	emails, err := s.loadAccountEmailsForOrg(ctx, s.store.DB(), principal.OrganizationID, sameOrgIDs)
	if err != nil {
		dl.Errorf("list workgroup members email lookup failed workgroup_id='%s' %s: %v", params.WorkgroupId, principalLogFields(principal), err)
		return &api.ListWorkgroupMembersInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}

	result := make(api.ListWorkgroupMembersResponse, 0, len(memberships))
	for i := range memberships {
		m := memberships[i]
		result = append(result, *mapWorkgroupMembership(&m, emails[m.AccountID]))
	}
	return &result, nil
}
