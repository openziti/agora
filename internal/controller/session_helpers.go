package controller

import (
	"context"
	"errors"

	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/internal/persistence"
)

var (
	errUnknownEnvironment    = errors.New("unknown environment")
	errSessionInvalidState   = errors.New("session invalid state")
	errNotSessionProvider    = errors.New("not session provider")
	errNotSessionParticipant = errors.New("not session participant")
)

// requireVisibleSession returns the session when the caller is either the
// provider or the consumer. Returns persistence.ErrNotFound otherwise.
func (s *Service) requireVisibleSession(ctx context.Context, principal *accountPrincipal, sessionID string) (*persistence.Session, error) {
	sess, err := s.store.Sessions.GetByID(ctx, s.store.DB(), sessionID)
	if err != nil {
		return nil, err
	}
	if sess.ProviderAccountID != principal.AccountID && sess.ConsumerAccountID != principal.AccountID {
		return nil, persistence.ErrNotFound
	}
	return sess, nil
}

func mapSession(sess *persistence.Session) *api.Session {
	out := &api.Session{
		ID:                     sess.ID,
		AdvertisementId:        sess.AdvertisementID,
		WorkgroupId:            sess.WorkgroupID,
		ProviderAccountId:      sess.ProviderAccountID,
		ProviderOrganizationId: sess.ProviderOrganizationID,
		ConsumerAccountId:      sess.ConsumerAccountID,
		ConsumerOrganizationId: sess.ConsumerOrganizationID,
		TunnelMode:             api.AdvertisementTunnelMode(sess.TunnelMode),
		State:                  api.SessionState(sess.State),
		ProposedAt:             sess.ProposedAt,
	}
	if sess.TunnelID != nil {
		out.TunnelId.SetTo(*sess.TunnelID)
	}
	if sess.CloseReason != nil {
		out.CloseReason.SetTo(api.SessionCloseReason(*sess.CloseReason))
	}
	if sess.CloseDetail != nil {
		out.CloseDetail.SetTo(*sess.CloseDetail)
	}
	if sess.ProposerMessage != nil {
		out.ProposerMessage.SetTo(*sess.ProposerMessage)
	}
	if sess.AcceptedAt != nil {
		out.AcceptedAt.SetTo(*sess.AcceptedAt)
	}
	if sess.ClosedAt != nil {
		out.ClosedAt.SetTo(*sess.ClosedAt)
	}
	if snap, err := mapSessionSnapshot(sess.ContractSnapshotJSON); err == nil {
		out.ContractSnapshot = snap
	}
	return out
}
