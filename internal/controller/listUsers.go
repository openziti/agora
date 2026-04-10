package controller

import (
	"context"
	"errors"

	"github.com/michaelquigley/df/dl"
	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/internal/persistence"
)

func (s *Service) ListUsers(ctx context.Context, params api.ListUsersParams) (api.ListUsersRes, error) {
	if _, err := requireAdminPrincipal(ctx); err != nil {
		dl.Warn("unauthorized list users request")
		return &api.ListUsersUnauthorized{Code: "unauthorized", Message: "unauthorized"}, nil
	}

	var organizationID *string
	if id, ok := params.OrganizationId.Get(); ok {
		dl.Debugf("listing users organization_id='%s' %s", id, adminLogFields())
		if _, err := s.store.Organizations.GetByID(ctx, s.store.DB(), id); err != nil {
			if errors.Is(err, persistence.ErrNotFound) {
				dl.Warnf("list users organization not found organization_id='%s'", id)
				return &api.ListUsersNotFound{Code: "not_found", Message: "organization not found"}, nil
			}
			dl.Errorf("list users organization lookup failed organization_id='%s': %v", id, err)
			return &api.ListUsersInternalServerError{Code: "internal_error", Message: err.Error()}, nil
		}
		organizationID = &id
	} else {
		dl.Debugf("listing users globally %s", adminLogFields())
	}

	accounts, err := s.store.Accounts.List(ctx, s.store.DB(), organizationID)
	if err != nil {
		dl.Errorf("list users failed: %v", err)
		return &api.ListUsersInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}
	resp := make(api.ListAccountsResponse, 0, len(accounts))
	for i := range accounts {
		resp = append(resp, *mapAccount(&accounts[i]))
	}
	dl.Infof("listed %d user(s)", len(resp))
	return &resp, nil
}
