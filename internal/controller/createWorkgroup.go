package controller

import (
	"context"
	"errors"

	"github.com/michaelquigley/df/dl"
	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/internal/persistence"
)

func (s *Service) CreateWorkgroup(ctx context.Context, req *api.CreateWorkgroupRequest) (api.CreateWorkgroupRes, error) {
	if _, err := requireAdminPrincipal(ctx); err != nil {
		return &api.CreateWorkgroupUnauthorized{Code: "unauthorized", Message: "unauthorized"}, nil
	}

	scope := persistence.WorkgroupScope(req.Scope)
	if scope != persistence.WorkgroupScopeIntraOrg && scope != persistence.WorkgroupScopeInterOrg {
		return &api.CreateWorkgroupBadRequest{Code: "invalid_request", Message: "scope must be 'intra-org' or 'inter-org'"}, nil
	}

	if scope == persistence.WorkgroupScopeIntraOrg && len(req.ParticipatingOrganizationIds) > 0 {
		return &api.CreateWorkgroupBadRequest{Code: "invalid_request", Message: "intra-org workgroups must not specify participatingOrganizationIds"}, nil
	}
	if scope == persistence.WorkgroupScopeInterOrg && len(req.ParticipatingOrganizationIds) == 0 {
		return &api.CreateWorkgroupBadRequest{Code: "invalid_request", Message: "inter-org workgroups must specify at least one participating organization"}, nil
	}
	for _, orgID := range req.ParticipatingOrganizationIds {
		if orgID == req.OwnerOrganizationId {
			return &api.CreateWorkgroupBadRequest{Code: "owner_in_participants", Message: "ownerOrganizationId must not appear in participatingOrganizationIds"}, nil
		}
	}

	owner, err := s.store.Organizations.GetByID(ctx, s.store.DB(), req.OwnerOrganizationId)
	if err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			return &api.CreateWorkgroupBadRequest{Code: "unknown_reference", Message: "owner organization not found"}, nil
		}
		return &api.CreateWorkgroupInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}
	for _, orgID := range req.ParticipatingOrganizationIds {
		if _, err := s.store.Organizations.GetByID(ctx, s.store.DB(), orgID); err != nil {
			if errors.Is(err, persistence.ErrNotFound) {
				return &api.CreateWorkgroupBadRequest{Code: "unknown_reference", Message: "participating organization not found: " + orgID}, nil
			}
			return &api.CreateWorkgroupInternalServerError{Code: "internal_error", Message: err.Error()}, nil
		}
	}

	initialAdmin, err := s.store.Accounts.GetByID(ctx, s.store.DB(), req.InitialAdminAccountId)
	if err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			return &api.CreateWorkgroupBadRequest{Code: "unknown_reference", Message: "initial admin account not found"}, nil
		}
		return &api.CreateWorkgroupInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}
	if initialAdmin.OrganizationID != owner.ID {
		return &api.CreateWorkgroupBadRequest{Code: "cross_org_initial_admin", Message: "initialAdminAccountId must belong to ownerOrganizationId"}, nil
	}

	if existing, err := s.store.Workgroups.GetByNameInOrg(ctx, s.store.DB(), owner.ID, req.Name); err == nil && existing != nil {
		return &api.CreateWorkgroupConflict{Code: "name_in_use", Message: "workgroup name already exists in this organization"}, nil
	} else if err != nil && !errors.Is(err, persistence.ErrNotFound) {
		return &api.CreateWorkgroupInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}

	desiredState := persistence.WorkgroupStateActive
	if scope == persistence.WorkgroupScopeInterOrg {
		desiredState = persistence.WorkgroupStatePending
	}

	wgRecord := persistence.Workgroup{
		OwnerOrganizationID: owner.ID,
		Name:                req.Name,
		Scope:               scope,
		State:               desiredState,
	}
	if req.Description.Set {
		v := req.Description.Value
		wgRecord.Description = &v
	}

	var (
		created     *persistence.Workgroup
		invitations []persistence.WorkgroupInvitation
	)
	if err := s.store.WithTx(ctx, func(tx persistence.Queryer) error {
		var err error
		created, err = s.store.Workgroups.Create(ctx, tx, wgRecord)
		if err != nil {
			return err
		}
		for _, orgID := range req.ParticipatingOrganizationIds {
			inv, err := s.store.WorkgroupInvitations.Create(ctx, tx, persistence.WorkgroupInvitation{
				WorkgroupID:    created.ID,
				OrganizationID: orgID,
				State:          persistence.WorkgroupInvitationStatePending,
			})
			if err != nil {
				return err
			}
			invitations = append(invitations, *inv)
		}
		_, err = s.store.WorkgroupMemberships.Create(ctx, tx, persistence.WorkgroupMembership{
			WorkgroupID:    created.ID,
			OrganizationID: initialAdmin.OrganizationID,
			AccountID:      initialAdmin.ID,
			Role:           persistence.WorkgroupMembershipRoleAdmin,
		})
		return err
	}); err != nil {
		dl.Errorf("create workgroup persistence failed: %v", err)
		return &api.CreateWorkgroupConflict{Code: "conflict", Message: err.Error()}, nil
	}

	participating := make([]string, 0, len(invitations))
	for _, inv := range invitations {
		participating = append(participating, inv.OrganizationID)
	}

	resp := &api.CreateWorkgroupResponse{
		Workgroup:   *mapWorkgroup(created, participating),
		Invitations: make([]api.WorkgroupInvitation, 0, len(invitations)),
	}
	for i := range invitations {
		resp.Invitations = append(resp.Invitations, *mapWorkgroupInvitation(&invitations[i]))
	}
	dl.Infof("created workgroup workgroup_id='%s' name='%s' scope='%s' state='%s' owner_organization_id='%s' invitations=%d", created.ID, created.Name, created.Scope, created.State, created.OwnerOrganizationID, len(invitations))
	return resp, nil
}
