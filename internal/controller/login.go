package controller

import (
	"context"
	"errors"

	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/internal/persistence"
)

func (s *Service) Login(ctx context.Context, req *api.LoginRequest) (api.LoginRes, error) {
	acct, err := s.store.Accounts.FindByEmail(ctx, s.store.DB(), req.Email)
	if err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			return &api.LoginUnauthorized{Code: "unauthorized", Message: "invalid credentials"}, nil
		}
		return &api.LoginInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}
	ok, err := verifyPassword(req.Password, acct.PasswordSalt, acct.PasswordHash)
	if err != nil || !ok {
		return &api.LoginUnauthorized{Code: "unauthorized", Message: "invalid credentials"}, nil
	}
	return &api.AccountTokenResponse{AccountToken: acct.AccountToken}, nil
}
