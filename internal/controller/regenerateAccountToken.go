package controller

import (
	"context"

	"github.com/michaelquigley/df/dl"
	"github.com/openziti/agora/internal/api"
)

func (s *Service) RegenerateAccountToken(ctx context.Context, req *api.RegenerateTokenRequest) (api.RegenerateAccountTokenRes, error) {
	principal, err := requireAccountPrincipal(ctx)
	if err != nil {
		dl.Warnf("unauthorized account token regeneration request email='%s'", normalizeEmail(req.Email))
		return &api.RegenerateAccountTokenUnauthorized{Code: "unauthorized", Message: "unauthorized"}, nil
	}
	dl.Infof("regenerating account token requested_email='%s' %s", normalizeEmail(req.Email), principalLogFields(principal))
	acct, err := s.store.Accounts.FindByEmail(ctx, s.store.DB(), req.Email)
	if err != nil {
		dl.Warnf("regenerate account token account not found requested_email='%s' %s", normalizeEmail(req.Email), principalLogFields(principal))
		return &api.RegenerateAccountTokenNotFound{Code: "not_found", Message: "account not found"}, nil
	}
	if acct.ID != principal.AccountID {
		dl.Warnf("regenerate account token rejected requested_email='%s' %s", normalizeEmail(req.Email), principalLogFields(principal))
		return &api.RegenerateAccountTokenUnauthorized{Code: "unauthorized", Message: "unauthorized"}, nil
	}
	token := newToken()
	if err := s.store.Accounts.RotateToken(ctx, s.store.DB(), acct.ID, token); err != nil {
		dl.Errorf("regenerate account token failed %s: %v", principalLogFields(principal), err)
		return &api.RegenerateAccountTokenInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}
	dl.Infof("regenerated account token %s", principalLogFields(principal))
	return &api.AccountTokenResponse{AccountToken: token}, nil
}
