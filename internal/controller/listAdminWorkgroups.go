package controller

import (
	"context"

	"github.com/michaelquigley/df/dl"
	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/internal/persistence"
)

func (s *Service) ListAdminWorkgroups(ctx context.Context, params api.ListAdminWorkgroupsParams) (api.ListAdminWorkgroupsRes, error) {
	if _, err := requireAdminPrincipal(ctx); err != nil {
		return &api.ListAdminWorkgroupsUnauthorized{Code: "unauthorized", Message: "unauthorized"}, nil
	}

	state := persistence.WorkgroupState("")
	if params.State.Set {
		state = persistence.WorkgroupState(params.State.Value)
	}
	owner := ""
	if params.OwnerOrganizationId.Set {
		owner = params.OwnerOrganizationId.Value
	}
	invited := ""
	if params.InvitedOrganizationId.Set {
		invited = params.InvitedOrganizationId.Value
	}

	workgroups, err := s.store.Workgroups.ListForAdmin(ctx, s.store.DB(), owner, invited, state)
	if err != nil {
		dl.Errorf("admin list workgroups failed: %v", err)
		return &api.ListAdminWorkgroupsInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}

	result := make(api.ListAdminWorkgroupsResponse, 0, len(workgroups))
	for i := range workgroups {
		wg := workgroups[i]
		invs, err := s.store.WorkgroupInvitations.ListByWorkgroup(ctx, s.store.DB(), wg.ID)
		if err != nil {
			return &api.ListAdminWorkgroupsInternalServerError{Code: "internal_error", Message: err.Error()}, nil
		}
		participating := make([]string, 0, len(invs))
		invitations := make([]api.WorkgroupInvitation, 0, len(invs))
		for j := range invs {
			participating = append(participating, invs[j].OrganizationID)
			invitations = append(invitations, *mapWorkgroupInvitation(&invs[j]))
		}
		result = append(result, api.AdminWorkgroupEntry{
			Workgroup:   *mapWorkgroup(&wg, participating),
			Invitations: invitations,
		})
	}
	return &result, nil
}
