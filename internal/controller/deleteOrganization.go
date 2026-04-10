package controller

import (
	"context"
	"errors"

	"github.com/michaelquigley/df/dl"
	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/internal/persistence"
)

func (s *Service) DeleteOrganization(ctx context.Context, params api.DeleteOrganizationParams) (api.DeleteOrganizationRes, error) {
	if _, err := requireAdminPrincipal(ctx); err != nil {
		dl.Warn("unauthorized delete organization request")
		return &api.DeleteOrganizationUnauthorized{Code: "unauthorized", Message: "unauthorized"}, nil
	}
	dl.Infof("deleting organization organization_id='%s' %s", params.OrganizationId, adminLogFields())
	if err := s.store.Organizations.Delete(ctx, s.store.DB(), params.OrganizationId); err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			dl.Warnf("delete organization not found organization_id='%s'", params.OrganizationId)
			return &api.DeleteOrganizationNotFound{Code: "not_found", Message: "organization not found"}, nil
		}
		if errors.Is(err, persistence.ErrConflict) {
			dl.Warnf("delete organization conflict organization_id='%s': dependent resources remain", params.OrganizationId)
			return &api.DeleteOrganizationConflict{Code: "conflict", Message: "organization still has dependent resources"}, nil
		}
		dl.Errorf("delete organization failed organization_id='%s': %v", params.OrganizationId, err)
		return &api.DeleteOrganizationInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}
	dl.Infof("deleted organization organization_id='%s'", params.OrganizationId)
	return &api.DeleteOrganizationNoContent{}, nil
}
