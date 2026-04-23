package controller

import (
	"context"
	"errors"
	"time"

	"github.com/michaelquigley/df/dl"
	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/internal/persistence"
)

func (s *Service) AcceptWorkgroupInvitation(ctx context.Context, req *api.AcceptWorkgroupInvitationRequest, params api.AcceptWorkgroupInvitationParams) (api.AcceptWorkgroupInvitationRes, error) {
	if _, err := requireAdminPrincipal(ctx); err != nil {
		return &api.AcceptWorkgroupInvitationUnauthorized{Code: "unauthorized", Message: "unauthorized"}, nil
	}

	wg, err := s.store.Workgroups.GetByID(ctx, s.store.DB(), params.WorkgroupId)
	if err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			return &api.AcceptWorkgroupInvitationNotFound{Code: "not_found", Message: "workgroup not found"}, nil
		}
		return &api.AcceptWorkgroupInvitationInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}
	if wg.State == persistence.WorkgroupStateDeclined {
		return &api.AcceptWorkgroupInvitationConflict{Code: "workgroup_declined", Message: "workgroup is already declined"}, nil
	}

	invitation, err := s.store.WorkgroupInvitations.GetByWorkgroupAndOrg(ctx, s.store.DB(), wg.ID, params.OrganizationId)
	if err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			return &api.AcceptWorkgroupInvitationNotFound{Code: "not_found", Message: "invitation not found"}, nil
		}
		return &api.AcceptWorkgroupInvitationInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}
	if invitation.State != persistence.WorkgroupInvitationStatePending {
		return &api.AcceptWorkgroupInvitationConflict{Code: "already_acknowledged", Message: "invitation has already been acknowledged"}, nil
	}

	initialAdmin, err := s.store.Accounts.GetByID(ctx, s.store.DB(), req.InitialAdminAccountId)
	if err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			return &api.AcceptWorkgroupInvitationBadRequest{Code: "unknown_reference", Message: "initial admin account not found"}, nil
		}
		return &api.AcceptWorkgroupInvitationInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}
	if initialAdmin.OrganizationID != params.OrganizationId {
		return &api.AcceptWorkgroupInvitationBadRequest{Code: "cross_org_initial_admin", Message: "initialAdminAccountId must belong to the acknowledging organization"}, nil
	}

	acknowledgingAdmin := "admin"
	if req.AcknowledgingAdminAccountId.Set {
		acknowledgingAdmin = req.AcknowledgingAdminAccountId.Value
	}

	var newState persistence.WorkgroupState
	if err := s.store.WithTx(ctx, func(tx persistence.Queryer) error {
		if err := s.store.WorkgroupInvitations.UpdateState(ctx, tx, invitation.ID, persistence.WorkgroupInvitationStateAccepted, acknowledgingAdmin, time.Now().UTC()); err != nil {
			return err
		}
		if _, err := s.store.WorkgroupMemberships.Create(ctx, tx, persistence.WorkgroupMembership{
			WorkgroupID:    wg.ID,
			OrganizationID: initialAdmin.OrganizationID,
			AccountID:      initialAdmin.ID,
			Role:           persistence.WorkgroupMembershipRoleAdmin,
		}); err != nil {
			return err
		}
		state, err := s.recomputeWorkgroupState(ctx, tx, wg)
		if err != nil {
			return err
		}
		newState = state
		return nil
	}); err != nil {
		dl.Errorf("accept workgroup invitation failed workgroup_id='%s' organization_id='%s': %v", wg.ID, params.OrganizationId, err)
		return &api.AcceptWorkgroupInvitationInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}

	updatedInv, err := s.store.WorkgroupInvitations.GetByID(ctx, s.store.DB(), invitation.ID)
	if err != nil {
		return &api.AcceptWorkgroupInvitationInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}
	wg.State = newState
	participating, err := s.participatingOrgIDs(ctx, s.store.DB(), wg)
	if err != nil {
		return &api.AcceptWorkgroupInvitationInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}
	dl.Infof("accepted workgroup invitation workgroup_id='%s' organization_id='%s' workgroup_state='%s' acknowledged_by='%s'", wg.ID, params.OrganizationId, newState, acknowledgingAdmin)
	return &api.AcknowledgeWorkgroupInvitationResponse{
		Invitation: *mapWorkgroupInvitation(updatedInv),
		Workgroup:  *mapWorkgroup(wg, participating),
	}, nil
}
