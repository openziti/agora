package controller

import (
	"context"
	"errors"

	"github.com/michaelquigley/df/dl"
	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/internal/persistence"
)

func (s *Service) UpdateAdvertisement(ctx context.Context, req *api.UpdateAdvertisementRequest, params api.UpdateAdvertisementParams) (api.UpdateAdvertisementRes, error) {
	principal, err := requireAccountPrincipal(ctx)
	if err != nil {
		return &api.UpdateAdvertisementUnauthorized{Code: "unauthorized", Message: "unauthorized"}, nil
	}

	ad, err := s.requireOwnedAdvertisement(ctx, principal, params.AdvertisementId)
	if err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			return &api.UpdateAdvertisementNotFound{Code: "not_found", Message: "advertisement not found"}, nil
		}
		return &api.UpdateAdvertisementInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}
	if ad.Status == persistence.AdvertisementStatusRetracted {
		return &api.UpdateAdvertisementConflict{Code: "advertisement_retracted", Message: "cannot update a retracted advertisement"}, nil
	}

	updated := *ad

	if req.Name.Set {
		newName := req.Name.Value
		if newName != ad.Name {
			if existing, err := s.store.Advertisements.GetByAccountAndName(ctx, s.store.DB(), principal.AccountID, newName); err == nil && existing != nil && existing.ID != ad.ID {
				return &api.UpdateAdvertisementConflict{Code: "name_in_use", Message: "another advertisement of this account already uses that name"}, nil
			} else if err != nil && !errors.Is(err, persistence.ErrNotFound) {
				return &api.UpdateAdvertisementInternalServerError{Code: "internal_error", Message: err.Error()}, nil
			}
			updated.Name = newName
		}
	}
	if req.Description.Set {
		v := req.Description.Value
		updated.Description = &v
	}
	if req.Capabilities != nil {
		if err := validateCapabilities(req.Capabilities); err != nil {
			return &api.UpdateAdvertisementBadRequest{Code: "invalid_request", Message: err.Error()}, nil
		}
		updated.Capabilities = capabilitiesFromAPI(req.Capabilities)
	}
	if req.InteractionPatterns != nil {
		if err := validateInteractionPatterns(req.InteractionPatterns); err != nil {
			return &api.UpdateAdvertisementBadRequest{Code: "invalid_request", Message: err.Error()}, nil
		}
		updated.InteractionPatterns = interactionPatternsFromAPI(req.InteractionPatterns)
	}
	if req.WorkgroupScopes != nil {
		if len(req.WorkgroupScopes) == 0 {
			return &api.UpdateAdvertisementBadRequest{Code: "invalid_request", Message: "workgroupScopes must contain at least one workgroup"}, nil
		}
		if err := s.validateWorkgroupScopes(ctx, principal, req.WorkgroupScopes); err != nil {
			switch {
			case errors.Is(err, errUnknownWorkgroup):
				return &api.UpdateAdvertisementBadRequest{Code: "unknown_workgroup", Message: "one or more workgroupScopes does not exist"}, nil
			case errors.Is(err, errNotAWorkgroupMember):
				return &api.UpdateAdvertisementForbidden{Code: "not_a_workgroup_member", Message: "caller is not a member of one or more of the listed workgroups"}, nil
			default:
				return &api.UpdateAdvertisementInternalServerError{Code: "internal_error", Message: err.Error()}, nil
			}
		}
		updated.WorkgroupScopes = req.WorkgroupScopes
	}
	if req.TunnelMode.Set {
		updated.TunnelMode = persistence.TunnelMode(req.TunnelMode.Value)
	}
	if req.ContractId.Set {
		if req.ContractId.Value == "" {
			updated.ContractID = nil
		} else {
			if err := s.validateContractOwnership(ctx, principal, req.ContractId.Value); err != nil {
				if errors.Is(err, errUnknownContract) {
					return &api.UpdateAdvertisementBadRequest{Code: "unknown_contract", Message: "contract does not exist or is not owned by this account"}, nil
				}
				return &api.UpdateAdvertisementInternalServerError{Code: "internal_error", Message: err.Error()}, nil
			}
			v := req.ContractId.Value
			updated.ContractID = &v
		}
	}

	saved, err := s.store.Advertisements.Update(ctx, s.store.DB(), updated)
	if err != nil {
		dl.Errorf("update advertisement persistence failed id='%s' %s: %v", ad.ID, principalLogFields(principal), err)
		return &api.UpdateAdvertisementInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}
	dl.Infof("updated advertisement id='%s' %s", saved.ID, principalLogFields(principal))
	return mapAdvertisement(saved, []string(saved.WorkgroupScopes)), nil
}
