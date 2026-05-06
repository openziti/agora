package controller

import (
	"context"
	"errors"

	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/internal/persistence"
)

func (s *Service) GetSession(ctx context.Context, params api.GetSessionParams) (api.GetSessionRes, error) {
	principal, err := requireAccountPrincipal(ctx)
	if err != nil {
		return &api.GetSessionUnauthorized{Code: "unauthorized", Message: "unauthorized"}, nil
	}

	sess, err := s.requireVisibleSession(ctx, principal, params.SessionId)
	if err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			return &api.GetSessionNotFound{Code: "not_found", Message: "session not found"}, nil
		}
		return &api.GetSessionInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}
	return mapSession(sess, principal.OrganizationID), nil
}
