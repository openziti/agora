package controller

import (
	"context"
	"errors"

	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/internal/persistence"
)

func (s *Service) DeleteAccount(ctx context.Context, params api.DeleteAccountParams) (api.DeleteAccountRes, error) {
	if _, err := requireAdminPrincipal(ctx); err != nil {
		return &api.DeleteAccountUnauthorized{Code: "unauthorized", Message: "unauthorized"}, nil
	}
	if err := s.store.Accounts.Delete(ctx, s.store.DB(), params.OrganizationId, params.AccountId); err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			return &api.DeleteAccountNotFound{Code: "not_found", Message: "account not found"}, nil
		}
		if errors.Is(err, persistence.ErrConflict) {
			return &api.DeleteAccountConflict{Code: "conflict", Message: "account still has dependent resources"}, nil
		}
		return &api.DeleteAccountInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}
	return &api.DeleteAccountNoContent{}, nil
}
