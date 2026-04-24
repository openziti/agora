package controller

import (
	"context"
	"errors"
	"fmt"

	"github.com/michaelquigley/df/dl"
	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/internal/persistence"
)

func (s *Service) RejectSession(ctx context.Context, req api.OptCloseSessionRequest, params api.RejectSessionParams) (api.RejectSessionRes, error) {
	principal, err := requireAccountPrincipal(ctx)
	if err != nil {
		return &api.RejectSessionUnauthorized{Code: "unauthorized", Message: "unauthorized"}, nil
	}

	sess, err := s.store.Sessions.GetByID(ctx, s.store.DB(), params.SessionId)
	if err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			return &api.RejectSessionNotFound{Code: "not_found", Message: "session not found"}, nil
		}
		return &api.RejectSessionInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}
	if sess.ProviderAccountID != principal.AccountID {
		if sess.ConsumerAccountID != principal.AccountID {
			return &api.RejectSessionNotFound{Code: "not_found", Message: "session not found"}, nil
		}
		return &api.RejectSessionForbidden{Code: "not_provider", Message: "only the advertisement owner may reject this session"}, nil
	}
	if sess.State != persistence.SessionStateProposed {
		return &api.RejectSessionConflict{Code: "invalid_state", Message: fmt.Sprintf("session is in state '%s'", sess.State)}, nil
	}

	detail := ""
	if req.Set && req.Value.Reason.Set {
		detail = req.Value.Reason.Value
	}
	if _, err := s.store.Sessions.MarkRejected(ctx, s.store.DB(), sess.ID, detail); err != nil {
		return &api.RejectSessionInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}
	dl.Infof("rejected session id='%s' %s", sess.ID, principalLogFields(principal))
	return &api.RejectSessionNoContent{}, nil
}
