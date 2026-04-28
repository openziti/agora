package controller

import (
	"context"
	"errors"

	"github.com/lib/pq"
	"github.com/michaelquigley/df/dl"
	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/internal/persistence"
)

func (s *Service) UpdateContract(ctx context.Context, req *api.UpdateContractRequest, params api.UpdateContractParams) (api.UpdateContractRes, error) {
	principal, err := requireAccountPrincipal(ctx)
	if err != nil {
		return &api.UpdateContractUnauthorized{Code: "unauthorized", Message: "unauthorized"}, nil
	}
	c, err := s.requireOwnedContract(ctx, principal, params.ContractId)
	if err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			return &api.UpdateContractNotFound{Code: "not_found", Message: "contract not found"}, nil
		}
		return &api.UpdateContractInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}

	updated := *c

	if req.Name.Set {
		newName := req.Name.Value
		if newName != c.Name {
			if existing, err := s.store.Contracts.GetByAccountAndName(ctx, s.store.DB(), principal.AccountID, newName); err == nil && existing != nil && existing.ID != c.ID {
				return &api.UpdateContractConflict{Code: "name_in_use", Message: "another contract of this account already uses that name"}, nil
			} else if err != nil && !errors.Is(err, persistence.ErrNotFound) {
				return &api.UpdateContractInternalServerError{Code: "internal_error", Message: err.Error()}, nil
			}
			updated.Name = newName
		}
	}
	if req.Description.Set {
		v := req.Description.Value
		updated.Description = &v
	}
	if req.MaxDurationSeconds.Set {
		updated.MaxDurationSeconds = req.MaxDurationSeconds.Value
	}
	if req.MaxEnvelopeCount.Set {
		updated.MaxEnvelopeCount = req.MaxEnvelopeCount.Value
	}
	if req.MaxEnvelopeBytes.Set {
		updated.MaxEnvelopeBytes = req.MaxEnvelopeBytes.Value
	}
	if req.AllowedMessageTypes != nil {
		updated.AllowedMessageTypes = pq.StringArray(sanitizeStringList(req.AllowedMessageTypes))
	}
	if req.RequiredWorkgroupMemberships != nil {
		scopes := sanitizeStringList(req.RequiredWorkgroupMemberships)
		if err := s.validateWorkgroupScopes(ctx, principal, scopes); err != nil {
			switch {
			case errors.Is(err, errUnknownWorkgroup):
				return &api.UpdateContractBadRequest{Code: "unknown_workgroup", Message: "one or more required workgroup memberships does not exist"}, nil
			case errors.Is(err, errNotAWorkgroupMember):
				return &api.UpdateContractForbidden{Code: "not_a_workgroup_member", Message: "caller is not a member of one or more required workgroups"}, nil
			default:
				return &api.UpdateContractInternalServerError{Code: "internal_error", Message: err.Error()}, nil
			}
		}
		updated.RequiredWorkgroupMemberships = pq.StringArray(scopes)
	}
	if req.MaturityRequirements.Set {
		maturity := persistence.MaturityRequirementsJSON{}
		if req.MaturityRequirements.Value.MinAccountAgeDays.Set {
			maturity.MinAccountAgeDays = req.MaturityRequirements.Value.MinAccountAgeDays.Value
		}
		updated.MaturityRequirements = maturity
	}
	if req.AccessMode.Set {
		updated.AccessMode = persistence.ContractAccessMode(req.AccessMode.Value)
	}

	saved, err := s.store.Contracts.Update(ctx, s.store.DB(), updated)
	if err != nil {
		dl.Errorf("update contract persistence failed id='%s' %s: %v", c.ID, principalLogFields(principal), err)
		return &api.UpdateContractInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}
	dl.Infof("updated contract id='%s' %s", saved.ID, principalLogFields(principal))
	return mapContract(saved), nil
}
