package controller

import (
	"context"
	"errors"

	"github.com/michaelquigley/df/dl"
	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/internal/persistence"
)

func (s *Service) PublishAdvertisement(ctx context.Context, req *api.PublishAdvertisementRequest) (api.PublishAdvertisementRes, error) {
	principal, err := requireAccountPrincipal(ctx)
	if err != nil {
		return &api.PublishAdvertisementUnauthorized{Code: "unauthorized", Message: "unauthorized"}, nil
	}

	if err := validateCapabilities(req.Capabilities); err != nil {
		return &api.PublishAdvertisementBadRequest{Code: "invalid_request", Message: err.Error()}, nil
	}
	if err := validateInteractionPatterns(req.InteractionPatterns); err != nil {
		return &api.PublishAdvertisementBadRequest{Code: "invalid_request", Message: err.Error()}, nil
	}
	if len(req.WorkgroupScopes) == 0 {
		return &api.PublishAdvertisementBadRequest{Code: "invalid_request", Message: "workgroupScopes must contain at least one workgroup"}, nil
	}

	if err := s.validateWorkgroupScopes(ctx, principal, req.WorkgroupScopes); err != nil {
		switch {
		case errors.Is(err, errUnknownWorkgroup):
			return &api.PublishAdvertisementBadRequest{Code: "unknown_workgroup", Message: "one or more workgroupScopes does not exist"}, nil
		case errors.Is(err, errNotAWorkgroupMember):
			return &api.PublishAdvertisementForbidden{Code: "not_a_workgroup_member", Message: "caller is not a member of one or more of the listed workgroups"}, nil
		default:
			return &api.PublishAdvertisementInternalServerError{Code: "internal_error", Message: err.Error()}, nil
		}
	}

	if existing, err := s.store.Advertisements.GetByAccountAndName(ctx, s.store.DB(), principal.AccountID, req.Name); err == nil && existing != nil {
		return &api.PublishAdvertisementConflict{Code: "name_in_use", Message: "advertisement name already in use by this account"}, nil
	} else if err != nil && !errors.Is(err, persistence.ErrNotFound) {
		return &api.PublishAdvertisementInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}

	ad := persistence.Advertisement{
		OrganizationID:      principal.OrganizationID,
		AccountID:           principal.AccountID,
		Name:                req.Name,
		Capabilities:        capabilitiesFromAPI(req.Capabilities),
		InteractionPatterns: interactionPatternsFromAPI(req.InteractionPatterns),
		WorkgroupScopes:     req.WorkgroupScopes,
		TunnelMode:          persistence.TunnelModeTCP,
		Status:              persistence.AdvertisementStatusActive,
	}
	if req.Description.Set {
		v := req.Description.Value
		ad.Description = &v
	}
	if req.TunnelMode.Set {
		ad.TunnelMode = persistence.TunnelMode(req.TunnelMode.Value)
	}

	created, err := s.store.Advertisements.Create(ctx, s.store.DB(), ad)
	if err != nil {
		dl.Errorf("publish advertisement persistence failed name='%s' %s: %v", req.Name, principalLogFields(principal), err)
		return &api.PublishAdvertisementInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}
	dl.Infof("published advertisement id='%s' name='%s' %s", created.ID, created.Name, principalLogFields(principal))
	return mapAdvertisement(created, []string(created.WorkgroupScopes)), nil
}
