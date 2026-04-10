package controller

import (
	"context"

	"github.com/michaelquigley/df/dl"
	"github.com/openziti/agora/internal/api"
)

func (s *Service) ListAccounts(ctx context.Context, params api.ListAccountsParams) (api.ListAccountsRes, error) {
	if _, err := requireAdminPrincipal(ctx); err != nil {
		dl.Warn("unauthorized list accounts request")
		return &api.ListAccountsUnauthorized{Code: "unauthorized", Message: "unauthorized"}, nil
	}
	dl.Debugf("listing accounts organization_id='%s' %s", params.OrganizationId, adminLogFields())
	if _, err := s.store.Organizations.GetByID(ctx, s.store.DB(), params.OrganizationId); err != nil {
		dl.Warnf("list accounts organization not found organization_id='%s': %v", params.OrganizationId, err)
		return &api.ListAccountsNotFound{Code: "not_found", Message: "organization not found"}, nil
	}
	accounts, err := s.store.Accounts.ListByOrganization(ctx, s.store.DB(), params.OrganizationId)
	if err != nil {
		dl.Errorf("list accounts failed organization_id='%s': %v", params.OrganizationId, err)
		return &api.ListAccountsInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}
	resp := make(api.ListAccountsResponse, 0, len(accounts))
	for i := range accounts {
		resp = append(resp, *mapAccount(&accounts[i]))
	}
	dl.Infof("listed %d account(s) for organization_id='%s'", len(resp), params.OrganizationId)
	return &resp, nil
}
