package controller

import (
	"context"
	"errors"

	"github.com/michaelquigley/df/dl"
	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/internal/persistence"
)

func (s *Service) Login(ctx context.Context, req *api.LoginRequest) (api.LoginRes, error) {
	email := normalizeEmail(req.Email)
	dl.Infof("login attempt email='%s'", email)
	acct, err := s.store.Accounts.FindByEmail(ctx, s.store.DB(), req.Email)
	if err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			dl.Warnf("login rejected email='%s': account not found", email)
			return &api.LoginUnauthorized{Code: "unauthorized", Message: "invalid credentials"}, nil
		}
		dl.Errorf("login lookup failed email='%s': %v", email, err)
		return &api.LoginInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}
	ok, err := verifyPassword(req.Password, acct.PasswordSalt, acct.PasswordHash)
	if err != nil || !ok {
		if err != nil {
			dl.Errorf("login password verification failed email='%s': %v", email, err)
		} else {
			dl.Warnf("login rejected email='%s': invalid credentials", email)
		}
		s.recordAccountLoginFailedFailOpen(ctx, acct, email)
		return &api.LoginUnauthorized{Code: "unauthorized", Message: "invalid credentials"}, nil
	}
	dl.Infof("login succeeded email='%s' account_id='%s' organization_id='%s'", acct.Email, acct.ID, acct.OrganizationID)
	s.recordAccountLoginFailOpen(ctx, acct)
	return &api.AccountTokenResponse{AccountToken: acct.AccountToken}, nil
}
