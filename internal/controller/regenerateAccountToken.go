package controller

import (
	"context"

	"github.com/openziti/agora/internal/api"
)

func (s *Service) RegenerateAccountToken(ctx context.Context, req *api.RegenerateTokenRequest) (api.RegenerateAccountTokenRes, error) {
	principal, err := requireAccountPrincipal(ctx)
	if err != nil {
		return &api.RegenerateAccountTokenUnauthorized{Code: "unauthorized", Message: "unauthorized"}, nil
	}
	acct, err := s.store.Accounts.FindByEmail(ctx, s.store.DB(), req.Email)
	if err != nil {
		return &api.RegenerateAccountTokenNotFound{Code: "not_found", Message: "account not found"}, nil
	}
	if acct.ID != principal.AccountID {
		return &api.RegenerateAccountTokenUnauthorized{Code: "unauthorized", Message: "unauthorized"}, nil
	}
	token := newToken()
	if err := s.store.Accounts.RotateToken(ctx, s.store.DB(), acct.ID, token); err != nil {
		return &api.RegenerateAccountTokenInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}
	return &api.AccountTokenResponse{AccountToken: token}, nil
}
