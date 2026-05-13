package controller

import (
	"context"

	"github.com/michaelquigley/df/dl"
	"github.com/openziti/agora/internal/api"
)

func (s *Service) GetAccountToken(ctx context.Context) (api.GetAccountTokenRes, error) {
	principal, err := requireAccountPrincipal(ctx)
	if err != nil {
		return &api.GetAccountTokenUnauthorized{Code: "unauthorized", Message: "unauthorized"}, nil
	}
	acct, err := s.store.Accounts.GetByID(ctx, s.store.DB(), principal.AccountID)
	if err != nil {
		dl.Errorf("get account token: account lookup failed %s: %v", principalLogFields(principal), err)
		return &api.GetAccountTokenInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}
	dl.Infof("account token revealed %s", principalLogFields(principal))
	return &api.AccountTokenResponse{AccountToken: acct.AccountToken}, nil
}
