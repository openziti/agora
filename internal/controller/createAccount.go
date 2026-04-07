package controller

import (
	"context"

	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/internal/persistence"
)

func (s *Service) CreateAccount(ctx context.Context, req *api.CreateAccountRequest, params api.CreateAccountParams) (api.CreateAccountRes, error) {
	if _, err := requireAdminPrincipal(ctx); err != nil {
		return &api.CreateAccountUnauthorized{Code: "unauthorized", Message: "unauthorized"}, nil
	}
	if _, err := s.store.Organizations.GetByID(ctx, s.store.DB(), params.OrganizationId); err != nil {
		return &api.CreateAccountNotFound{Code: "not_found", Message: "organization not found"}, nil
	}
	hashed, err := hashPassword(req.Password)
	if err != nil {
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
		return &api.CreateAccountConflict{Code: "conflict", Message: err.Error()}, nil
	}
	return &api.AccountTokenResponse{AccountToken: created.AccountToken}, nil
}
