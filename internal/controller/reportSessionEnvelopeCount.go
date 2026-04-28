package controller

import (
	"context"
	"errors"

	"github.com/michaelquigley/df/dl"
	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/internal/persistence"
)

func (s *Service) ReportSessionEnvelopeCount(ctx context.Context, req *api.ReportEnvelopeCountRequest, params api.ReportSessionEnvelopeCountParams) (api.ReportSessionEnvelopeCountRes, error) {
	principal, err := requireAccountPrincipal(ctx)
	if err != nil {
		return &api.ReportSessionEnvelopeCountUnauthorized{Code: "unauthorized", Message: "unauthorized"}, nil
	}
	sess, err := s.store.Sessions.GetByID(ctx, s.store.DB(), params.SessionId)
	if err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			return &api.ReportSessionEnvelopeCountNotFound{Code: "not_found", Message: "session not found"}, nil
		}
		return &api.ReportSessionEnvelopeCountInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}
	if sess.ProviderAccountID != principal.AccountID {
		if sess.ConsumerAccountID != principal.AccountID {
			return &api.ReportSessionEnvelopeCountNotFound{Code: "not_found", Message: "session not found"}, nil
		}
		return &api.ReportSessionEnvelopeCountForbidden{Code: "not_provider", Message: "only the provider may report envelope counts"}, nil
	}
	if req.Count < 0 {
		return &api.ReportSessionEnvelopeCountBadRequest{Code: "invalid_request", Message: "count must be non-negative"}, nil
	}
	if err := s.store.Sessions.RecordEnvelopeCount(ctx, s.store.DB(), sess.ID, req.Count); err != nil {
		dl.Errorf("record envelope count failed session_id='%s' count=%d %s: %v", sess.ID, req.Count, principalLogFields(principal), err)
		return &api.ReportSessionEnvelopeCountInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}
	return &api.ReportSessionEnvelopeCountNoContent{}, nil
}
