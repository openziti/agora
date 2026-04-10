package controller

import (
	"context"

	"github.com/michaelquigley/df/dl"
	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/internal/persistence"
)

func (s *Service) CreateAccount(ctx context.Context, req *api.CreateAccountRequest, params api.CreateAccountParams) (api.CreateAccountRes, error) {
	if _, err := requireAdminPrincipal(ctx); err != nil {
		dl.Warn("unauthorized create account request")
		return &api.CreateAccountUnauthorized{Code: "unauthorized", Message: "unauthorized"}, nil
	}
	dl.Infof("creating account email='%s' organization_id='%s' %s", normalizeEmail(req.Email), params.OrganizationId, adminLogFields())
	if _, err := s.store.Organizations.GetByID(ctx, s.store.DB(), params.OrganizationId); err != nil {
		dl.Warnf("create account organization not found email='%s' organization_id='%s': %v", normalizeEmail(req.Email), params.OrganizationId, err)
		return &api.CreateAccountNotFound{Code: "not_found", Message: "organization not found"}, nil
	}
	hashed, err := hashPassword(req.Password)
	if err != nil {
		dl.Errorf("hash password failed for create account email='%s' organization_id='%s': %v", normalizeEmail(req.Email), params.OrganizationId, err)
		return &api.CreateAccountInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}
	account := persistence.Account{
		OrganizationID: params.OrganizationId,
		Email:          normalizeEmail(req.Email),
		PasswordSalt:   hashed.Salt,
		PasswordHash:   hashed.Hash,
		AccountToken:   newToken(),
	}
	if displayName, ok := req.DisplayName.Get(); ok {
		account.DisplayName = &displayName
	}
	if role, ok := req.Role.Get(); ok {
		account.Role = persistence.AccountRole(role)
	}
	if status, ok := req.Status.Get(); ok {
		account.Status = persistence.AccountStatus(status)
	}
	created, err := s.store.Accounts.Create(ctx, s.store.DB(), account)
	if err != nil {
		dl.Errorf("create account failed email='%s' organization_id='%s': %v", normalizeEmail(req.Email), params.OrganizationId, err)
		return &api.CreateAccountConflict{Code: "conflict", Message: err.Error()}, nil
	}
	dl.Infof("created account id='%s' email='%s' organization_id='%s'", created.ID, created.Email, created.OrganizationID)
	return &api.AccountTokenResponse{AccountToken: created.AccountToken}, nil
}
