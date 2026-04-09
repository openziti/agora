package controller

import (
	"context"
	"errors"

	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/internal/persistence"
)

func (s *Service) ListUsers(ctx context.Context, params api.ListUsersParams) (api.ListUsersRes, error) {
	if _, err := requireAdminPrincipal(ctx); err != nil {
		return &api.ListUsersUnauthorized{Code: "unauthorized", Message: "unauthorized"}, nil
	}

	var organizationID *string
	if id, ok := params.OrganizationId.Get(); ok {
		if _, err := s.store.Organizations.GetByID(ctx, s.store.DB(), id); err != nil {
			if errors.Is(err, persistence.ErrNotFound) {
				return &api.ListUsersNotFound{Code: "not_found", Message: "organization not found"}, nil
			}
			return &api.ListUsersInternalServerError{Code: "internal_error", Message: err.Error()}, nil
		}
		organizationID = &id
	}

	accounts, err := s.store.Accounts.List(ctx, s.store.DB(), organizationID)
	if err != nil {
		return &api.ListUsersInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}
	resp := make(api.ListAccountsResponse, 0, len(accounts))
	for i := range accounts {
		resp = append(resp, *mapAccount(&accounts[i]))
	}
	return &resp, nil
}
