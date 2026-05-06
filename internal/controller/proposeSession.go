package controller

import (
	"context"
	"errors"

	"github.com/michaelquigley/df/dl"
	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/internal/persistence"
)

func (s *Service) ProposeSession(ctx context.Context, req *api.ProposeSessionRequest) (api.ProposeSessionRes, error) {
	principal, err := requireAccountPrincipal(ctx)
	if err != nil {
		return &api.ProposeSessionUnauthorized{Code: "unauthorized", Message: "unauthorized"}, nil
	}

	ad, visibleScopes, err := s.requireVisibleAdvertisement(ctx, principal, req.AdvertisementId)
	if err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			return &api.ProposeSessionNotFound{Code: "not_found", Message: "advertisement not found"}, nil
		}
		return &api.ProposeSessionInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}
	if ad.Status == persistence.AdvertisementStatusRetracted {
		return &api.ProposeSessionConflict{Code: "advertisement_retracted", Message: "advertisement is retracted"}, nil
	}

	// workgroupId must be in visibleScopes (the caller's intersection with the ad's scopes).
	inScope := false
	for _, wg := range visibleScopes {
		if wg == req.WorkgroupId {
			inScope = true
			break
		}
	}
	if !inScope {
		return &api.ProposeSessionBadRequest{Code: "workgroup_not_in_scope", Message: "workgroupId is not in the advertisement's visible scopes for this caller"}, nil
	}

	sess := persistence.Session{
		AdvertisementID:        ad.ID,
		WorkgroupID:            req.WorkgroupId,
		ProviderAccountID:      ad.AccountID,
		ProviderOrganizationID: ad.OrganizationID,
		ConsumerAccountID:      principal.AccountID,
		ConsumerOrganizationID: principal.OrganizationID,
		TunnelMode:             ad.TunnelMode,
		State:                  persistence.SessionStateProposed,
	}
	if req.ProposerMessage.Set && req.ProposerMessage.Value != "" {
		v := req.ProposerMessage.Value
		sess.ProposerMessage = &v
	}

	var created *persistence.Session
	if err := s.store.WithTx(ctx, func(tx persistence.Queryer) error {
		var err error
		created, err = s.store.Sessions.Create(ctx, tx, sess)
		if err != nil {
			return err
		}
		return s.recordSessionProposed(ctx, tx, created)
	}); err != nil {
		dl.Errorf("propose session persistence failed advertisement_id='%s' %s: %v", ad.ID, principalLogFields(principal), err)
		return &api.ProposeSessionInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}
	dl.Infof("proposed session id='%s' advertisement_id='%s' workgroup_id='%s' %s",
		created.ID, created.AdvertisementID, created.WorkgroupID, principalLogFields(principal))
	createdWithDisplay, err := s.store.Sessions.GetByIDWithDisplay(ctx, s.store.DB(), created.ID)
	if err != nil {
		dl.Errorf("propose session display lookup failed session_id='%s' %s: %v", created.ID, principalLogFields(principal), err)
		return &api.ProposeSessionInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}
	return mapSession(createdWithDisplay, principal.OrganizationID), nil
}
