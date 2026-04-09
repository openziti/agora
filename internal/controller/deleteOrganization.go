package controller

import (
	"context"
	"errors"

	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/internal/persistence"
)

func (s *Service) DeleteOrganization(ctx context.Context, params api.DeleteOrganizationParams) (api.DeleteOrganizationRes, error) {
	if _, err := requireAdminPrincipal(ctx); err != nil {
		return &api.DeleteOrganizationUnauthorized{Code: "unauthorized", Message: "unauthorized"}, nil
	}
	if err := s.store.Organizations.Delete(ctx, s.store.DB(), params.OrganizationId); err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			return &api.DeleteOrganizationNotFound{Code: "not_found", Message: "organization not found"}, nil
		}
		if errors.Is(err, persistence.ErrConflict) {
			return &api.DeleteOrganizationConflict{Code: "conflict", Message: "organization still has dependent resources"}, nil
		}
		return &api.DeleteOrganizationInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}
	return &api.DeleteOrganizationNoContent{}, nil
}
