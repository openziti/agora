package controller

import (
	"context"
	"errors"

	"github.com/michaelquigley/df/dl"
	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/internal/persistence"
)

func (s *Service) AdminCloseSession(ctx context.Context, req api.OptCloseSessionRequest, params api.AdminCloseSessionParams) (api.AdminCloseSessionRes, error) {
	if _, err := requireAdminPrincipal(ctx); err != nil {
		return &api.AdminCloseSessionUnauthorized{Code: "unauthorized", Message: "unauthorized"}, nil
	}

	sess, err := s.store.Sessions.GetByID(ctx, s.store.DB(), params.SessionId)
	if err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			return &api.AdminCloseSessionNotFound{Code: "not_found", Message: "session not found"}, nil
		}
		return &api.AdminCloseSessionInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}

	detail := ""
	if req.Set && req.Value.Reason.Set {
		detail = req.Value.Reason.Value
	}
	if err := s.teardownSession(ctx, sess, persistence.SessionCloseReasonAdminClose, detail); err != nil {
		return &api.AdminCloseSessionInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}
	dl.Infof("admin-closed session id='%s'", sess.ID)
	return &api.AdminCloseSessionNoContent{}, nil
}
