package controller

import (
	"context"

	"github.com/openziti/agora/internal/api"
)

func (s *Service) ChangePassword(ctx context.Context, req *api.ChangePasswordRequest) (api.ChangePasswordRes, error) {
	principal, err := requireAccountPrincipal(ctx)
	if err != nil {
		return &api.ChangePasswordUnauthorized{Code: "unauthorized", Message: "unauthorized"}, nil
	}
	acct, err := s.store.Accounts.FindByEmail(ctx, s.store.DB(), req.Email)
	if err != nil {
		return &api.ChangePasswordNotFound{Code: "not_found", Message: "account not found"}, nil
	}
	if acct.ID != principal.AccountID {
		return &api.ChangePasswordUnauthorized{Code: "unauthorized", Message: "unauthorized"}, nil
	}
	ok, err := verifyPassword(req.OldPassword, acct.PasswordSalt, acct.PasswordHash)
	if err != nil || !ok {
		return &api.ChangePasswordUnauthorized{Code: "unauthorized", Message: "invalid credentials"}, nil
	}
	hashed, err := hashPassword(req.NewPassword)
	if err != nil {
		return &api.ChangePasswordInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}
	if err := s.store.Accounts.UpdatePassword(ctx, s.store.DB(), acct.ID, hashed.Salt, hashed.Hash); err != nil {
		return &api.ChangePasswordInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}
	return &api.ChangePasswordOK{}, nil
}
