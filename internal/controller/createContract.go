package controller

import (
	"context"
	"errors"

	"github.com/lib/pq"
	"github.com/michaelquigley/df/dl"
	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/internal/persistence"
)

func (s *Service) CreateContract(ctx context.Context, req *api.CreateContractRequest) (api.CreateContractRes, error) {
	principal, err := requireAccountPrincipal(ctx)
	if err != nil {
		return &api.CreateContractUnauthorized{Code: "unauthorized", Message: "unauthorized"}, nil
	}

	accessMode := persistence.ContractAccessModeApprovalRequired
	if req.AccessMode.Set {
		accessMode = persistence.ContractAccessMode(req.AccessMode.Value)
	}

	workgroupScopes := sanitizeStringList(req.RequiredWorkgroupMemberships)
	if err := s.validateWorkgroupScopes(ctx, principal, workgroupScopes); err != nil {
		switch {
		case errors.Is(err, errUnknownWorkgroup):
			return &api.CreateContractBadRequest{Code: "unknown_workgroup", Message: "one or more required workgroup memberships does not exist"}, nil
		case errors.Is(err, errNotAWorkgroupMember):
			return &api.CreateContractForbidden{Code: "not_a_workgroup_member", Message: "caller is not a member of one or more required workgroups"}, nil
		default:
			return &api.CreateContractInternalServerError{Code: "internal_error", Message: err.Error()}, nil
		}
	}

	if existing, err := s.store.Contracts.GetByAccountAndName(ctx, s.store.DB(), principal.AccountID, req.Name); err == nil && existing != nil {
		return &api.CreateContractConflict{Code: "name_in_use", Message: "contract name already in use by this account"}, nil
	} else if err != nil && !errors.Is(err, persistence.ErrNotFound) {
		return &api.CreateContractInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}

	c := persistence.Contract{
		OrganizationID:               principal.OrganizationID,
		AccountID:                    principal.AccountID,
		Name:                         req.Name,
		SchemaVersion:                1,
		AccessMode:                   accessMode,
		AllowedMessageTypes:          pq.StringArray(sanitizeStringList(req.AllowedMessageTypes)),
		RequiredWorkgroupMemberships: pq.StringArray(workgroupScopes),
	}
	if req.Description.Set {
		v := req.Description.Value
		c.Description = &v
	}
	if req.MaxDurationSeconds.Set {
		c.MaxDurationSeconds = req.MaxDurationSeconds.Value
	}
	if req.MaxEnvelopeCount.Set {
		c.MaxEnvelopeCount = req.MaxEnvelopeCount.Value
	}
	if req.MaturityRequirements.Set && req.MaturityRequirements.Value.MinAccountAgeDays.Set {
		c.MaturityRequirements = persistence.MaturityRequirementsJSON{
			MinAccountAgeDays: req.MaturityRequirements.Value.MinAccountAgeDays.Value,
		}
	}

	created, err := s.store.Contracts.Create(ctx, s.store.DB(), c)
	if err != nil {
		dl.Errorf("create contract persistence failed name='%s' %s: %v", req.Name, principalLogFields(principal), err)
		return &api.CreateContractInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}
	dl.Infof("created contract id='%s' name='%s' %s", created.ID, created.Name, principalLogFields(principal))
	return mapContract(created), nil
}
