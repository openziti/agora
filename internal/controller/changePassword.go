package controller

import (
	"context"

	"github.com/michaelquigley/df/dl"
	"github.com/openziti/agora/internal/api"
)

func (s *Service) ChangePassword(ctx context.Context, req *api.ChangePasswordRequest) (api.ChangePasswordRes, error) {
	principal, err := requireAccountPrincipal(ctx)
	if err != nil {
		dl.Warnf("unauthorized change password request email='%s'", normalizeEmail(req.Email))
		return &api.ChangePasswordUnauthorized{Code: "unauthorized", Message: "unauthorized"}, nil
	}
	dl.Infof("changing password requested_email='%s' %s", normalizeEmail(req.Email), principalLogFields(principal))
	acct, err := s.store.Accounts.FindByEmail(ctx, s.store.DB(), req.Email)
	if err != nil {
		dl.Warnf("change password account not found requested_email='%s' %s", normalizeEmail(req.Email), principalLogFields(principal))
		return &api.ChangePasswordNotFound{Code: "not_found", Message: "account not found"}, nil
	}
	if acct.ID != principal.AccountID {
		dl.Warnf("change password rejected requested_email='%s' %s", normalizeEmail(req.Email), principalLogFields(principal))
		return &api.ChangePasswordUnauthorized{Code: "unauthorized", Message: "unauthorized"}, nil
	}
	ok, err := verifyPassword(req.OldPassword, acct.PasswordSalt, acct.PasswordHash)
	if err != nil || !ok {
		if err != nil {
			dl.Errorf("change password verification failed %s: %v", principalLogFields(principal), err)
		} else {
			dl.Warnf("change password rejected due to invalid credentials %s", principalLogFields(principal))
		}
		return &api.ChangePasswordUnauthorized{Code: "unauthorized", Message: "invalid credentials"}, nil
	}
	hashed, err := hashPassword(req.NewPassword)
	if err != nil {
		dl.Errorf("change password hashing failed %s: %v", principalLogFields(principal), err)
		return &api.ChangePasswordInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}
	if err := s.store.Accounts.UpdatePassword(ctx, s.store.DB(), acct.ID, hashed.Salt, hashed.Hash); err != nil {
		dl.Errorf("change password update failed %s: %v", principalLogFields(principal), err)
		return &api.ChangePasswordInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}
	dl.Infof("changed password %s", principalLogFields(principal))
	return &api.ChangePasswordOK{}, nil
}
