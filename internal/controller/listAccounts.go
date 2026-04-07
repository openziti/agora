package controller

import (
	"context"

	"github.com/openziti/agora/internal/api"
)

func (s *Service) ListAccounts(ctx context.Context, params api.ListAccountsParams) (api.ListAccountsRes, error) {
	if _, err := requireAdminPrincipal(ctx); err != nil {
		return &api.ListAccountsUnauthorized{Code: "unauthorized", Message: "unauthorized"}, nil
	}
	if _, err := s.store.Organizations.GetByID(ctx, s.store.DB(), params.OrganizationId); err != nil {
		return &api.ListAccountsNotFound{Code: "not_found", Message: "organization not found"}, nil
	}
	accounts, err := s.store.Accounts.ListByOrganization(ctx, s.store.DB(), params.OrganizationId)
	if err != nil {
		return &api.ListAccountsInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}
	resp := make(api.ListAccountsResponse, 0, len(accounts))
	for i := range accounts {
		resp = append(resp, *mapAccount(&accounts[i]))
	}
	return &resp, nil
}
