package controller

import (
	"context"
	"errors"
	"time"

	"github.com/michaelquigley/df/dl"
	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/internal/persistence"
)

func (s *Service) DeclineWorkgroupInvitation(ctx context.Context, req *api.DeclineWorkgroupInvitationRequest, params api.DeclineWorkgroupInvitationParams) (api.DeclineWorkgroupInvitationRes, error) {
	if _, err := requireAdminPrincipal(ctx); err != nil {
		return &api.DeclineWorkgroupInvitationUnauthorized{Code: "unauthorized", Message: "unauthorized"}, nil
	}

	wg, err := s.store.Workgroups.GetByID(ctx, s.store.DB(), params.WorkgroupId)
	if err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			return &api.DeclineWorkgroupInvitationNotFound{Code: "not_found", Message: "workgroup not found"}, nil
		}
		return &api.DeclineWorkgroupInvitationInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}

	invitation, err := s.store.WorkgroupInvitations.GetByWorkgroupAndOrg(ctx, s.store.DB(), wg.ID, params.OrganizationId)
	if err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			return &api.DeclineWorkgroupInvitationNotFound{Code: "not_found", Message: "invitation not found"}, nil
		}
		return &api.DeclineWorkgroupInvitationInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}
	if invitation.State != persistence.WorkgroupInvitationStatePending {
		return &api.DeclineWorkgroupInvitationConflict{Code: "already_acknowledged", Message: "invitation has already been acknowledged"}, nil
	}

	acknowledgingAdmin := "admin"
	if req.AcknowledgingAdminAccountId.Set {
		acknowledgingAdmin = req.AcknowledgingAdminAccountId.Value
	}

	var newState persistence.WorkgroupState
	if err := s.store.WithTx(ctx, func(tx persistence.Queryer) error {
		if err := s.store.WorkgroupInvitations.UpdateState(ctx, tx, invitation.ID, persistence.WorkgroupInvitationStateDeclined, acknowledgingAdmin, time.Now().UTC()); err != nil {
			return err
		}
		state, err := s.recomputeWorkgroupState(ctx, tx, wg)
		if err != nil {
			return err
		}
		newState = state
		return nil
	}); err != nil {
		dl.Errorf("decline workgroup invitation failed workgroup_id='%s' organization_id='%s': %v", wg.ID, params.OrganizationId, err)
		return &api.DeclineWorkgroupInvitationInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}

	updatedInv, err := s.store.WorkgroupInvitations.GetByID(ctx, s.store.DB(), invitation.ID)
	if err != nil {
		return &api.DeclineWorkgroupInvitationInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}
	wg.State = newState
	participating, err := s.participatingOrgIDs(ctx, s.store.DB(), wg)
	if err != nil {
		return &api.DeclineWorkgroupInvitationInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}
	dl.Infof("declined workgroup invitation workgroup_id='%s' organization_id='%s' workgroup_state='%s' acknowledged_by='%s'", wg.ID, params.OrganizationId, newState, acknowledgingAdmin)
	return &api.AcknowledgeWorkgroupInvitationResponse{
		Invitation: *mapWorkgroupInvitation(updatedInv),
		Workgroup:  *mapWorkgroup(wg, participating),
	}, nil
}
