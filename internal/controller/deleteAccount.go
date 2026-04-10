package controller

import (
	"context"
	"errors"

	"github.com/michaelquigley/df/dl"
	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/internal/persistence"
)

func (s *Service) DeleteAccount(ctx context.Context, params api.DeleteAccountParams) (api.DeleteAccountRes, error) {
	if _, err := requireAdminPrincipal(ctx); err != nil {
		dl.Warn("unauthorized delete account request")
		return &api.DeleteAccountUnauthorized{Code: "unauthorized", Message: "unauthorized"}, nil
	}
	dl.Infof("deleting account account_id='%s' organization_id='%s' %s", params.AccountId, params.OrganizationId, adminLogFields())
	if err := s.store.Accounts.Delete(ctx, s.store.DB(), params.OrganizationId, params.AccountId); err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			dl.Warnf("delete account not found account_id='%s' organization_id='%s'", params.AccountId, params.OrganizationId)
			return &api.DeleteAccountNotFound{Code: "not_found", Message: "account not found"}, nil
		}
		if errors.Is(err, persistence.ErrConflict) {
			dl.Warnf("delete account conflict account_id='%s' organization_id='%s': dependent resources remain", params.AccountId, params.OrganizationId)
			return &api.DeleteAccountConflict{Code: "conflict", Message: "account still has dependent resources"}, nil
		}
		dl.Errorf("delete account failed account_id='%s' organization_id='%s': %v", params.AccountId, params.OrganizationId, err)
		return &api.DeleteAccountInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}
	dl.Infof("deleted account account_id='%s' organization_id='%s'", params.AccountId, params.OrganizationId)
	return &api.DeleteAccountNoContent{}, nil
}
