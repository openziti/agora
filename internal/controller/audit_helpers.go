package controller

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/michaelquigley/df/dl"
	"github.com/openziti/agora/internal/persistence"
)

type sessionAuditBuilder func(organizationID, accountID string) persistence.AuditEvent

func (s *Service) recordSessionProposed(ctx context.Context, q persistence.Queryer, sess *persistence.Session) error {
	return s.recordTwoPartySessionEvent(ctx, q, sess, func(organizationID, accountID string) persistence.AuditEvent {
		return persistence.NewSessionProposedEvent(*sess, organizationID, accountID)
	})
}

func (s *Service) recordSessionAccepted(ctx context.Context, q persistence.Queryer, sess *persistence.Session, contractID *string) error {
	return s.recordTwoPartySessionEvent(ctx, q, sess, func(organizationID, accountID string) persistence.AuditEvent {
		return persistence.NewSessionAcceptedEvent(*sess, organizationID, accountID, contractID)
	})
}

func (s *Service) recordSessionRejected(ctx context.Context, q persistence.Queryer, sess *persistence.Session, reason string) error {
	return s.recordTwoPartySessionEvent(ctx, q, sess, func(organizationID, accountID string) persistence.AuditEvent {
		return persistence.NewSessionRejectedEvent(*sess, organizationID, accountID, reason)
	})
}

func (s *Service) recordSessionClosed(ctx context.Context, q persistence.Queryer, sess *persistence.Session, reason persistence.SessionCloseReason, detail string) error {
	durationSeconds := sessionDurationSeconds(sess)
	violationDimension := sessionViolationDimension(reason, detail)
	return s.recordTwoPartySessionEvent(ctx, q, sess, func(organizationID, accountID string) persistence.AuditEvent {
		return persistence.NewSessionClosedEvent(*sess, organizationID, accountID, reason, detail, durationSeconds, violationDimension)
	})
}

func (s *Service) recordEnvelopeFlowed(ctx context.Context, q persistence.Queryer, sess *persistence.Session, countDelta, totalCount int) error {
	return s.recordTwoPartySessionEvent(ctx, q, sess, func(organizationID, accountID string) persistence.AuditEvent {
		return persistence.NewEnvelopeFlowedEvent(*sess, organizationID, accountID, countDelta, totalCount)
	})
}

func (s *Service) markSessionClosedWithAudit(ctx context.Context, q persistence.Queryer, sessionID string, reason persistence.SessionCloseReason, detail string) (*persistence.Session, error) {
	closed, err := s.store.Sessions.MarkClosed(ctx, q, sessionID, reason, detail)
	if err != nil {
		return nil, err
	}
	if err := s.recordSessionClosed(ctx, q, closed, reason, detail); err != nil {
		return nil, err
	}
	return closed, nil
}

func (s *Service) recordTwoPartySessionEvent(ctx context.Context, q persistence.Queryer, sess *persistence.Session, build sessionAuditBuilder) error {
	provider := build(sess.ProviderOrganizationID, sess.ProviderAccountID)
	occurredAt := provider.OccurredAt
	if err := s.store.AuditEvents.Record(ctx, q, provider); err != nil {
		return err
	}
	if sess.ProviderOrganizationID == sess.ConsumerOrganizationID {
		return nil
	}
	consumer := build(sess.ConsumerOrganizationID, sess.ConsumerAccountID)
	consumer.OccurredAt = occurredAt
	return s.store.AuditEvents.Record(ctx, q, consumer)
}

func (s *Service) recordTunnelAttached(ctx context.Context, q persistence.Queryer, attachment *persistence.TunnelAttachment) error {
	sessionID, err := s.auditSessionIDForTunnel(ctx, q, attachment.TunnelID)
	if err != nil {
		return err
	}
	return s.store.AuditEvents.Record(ctx, q, persistence.NewTunnelAttachedEvent(*attachment, sessionID))
}

func (s *Service) recordTunnelDetached(ctx context.Context, q persistence.Queryer, attachment persistence.TunnelAttachment, finalState persistence.TunnelAttachmentState) error {
	sessionID, err := s.auditSessionIDForTunnel(ctx, q, attachment.TunnelID)
	if err != nil {
		return err
	}
	return s.store.AuditEvents.Record(ctx, q, persistence.NewTunnelDetachedEvent(attachment, sessionID, finalState))
}

func (s *Service) recordAdvertisementPublished(ctx context.Context, q persistence.Queryer, ad *persistence.Advertisement) error {
	return s.store.AuditEvents.Record(ctx, q, persistence.NewAdvertisementPublishedEvent(*ad))
}

func (s *Service) recordAdvertisementRetracted(ctx context.Context, q persistence.Queryer, ad *persistence.Advertisement, reason string) error {
	return s.store.AuditEvents.Record(ctx, q, persistence.NewAdvertisementRetractedEvent(*ad, reason))
}

func (s *Service) recordEnvironmentHeartbeat(ctx context.Context, q persistence.Queryer, env *persistence.Environment, latencyMS int) error {
	return s.store.AuditEvents.Record(ctx, q, persistence.NewEnvironmentHeartbeatEvent(*env, latencyMS))
}

func (s *Service) recordAccountLoginFailOpen(ctx context.Context, acct *persistence.Account) {
	if err := s.store.AuditEvents.Record(ctx, s.store.DB(), persistence.NewAccountLoginEvent(*acct)); err != nil {
		dl.Warnf("account login audit failed account_id='%s' organization_id='%s': %v", acct.ID, acct.OrganizationID, err)
	}
}

func (s *Service) recordAccountLoginFailedFailOpen(ctx context.Context, acct *persistence.Account, emailAttempted string) {
	if err := s.store.AuditEvents.Record(ctx, s.store.DB(), persistence.NewAccountLoginFailedEvent(*acct, emailAttempted)); err != nil {
		dl.Warnf("account login_failed audit failed account_id='%s' organization_id='%s': %v", acct.ID, acct.OrganizationID, err)
	}
}

func (s *Service) auditSessionIDForTunnel(ctx context.Context, q persistence.Queryer, tunnelID string) (*string, error) {
	sess, err := s.store.Sessions.GetByTunnelID(ctx, q, tunnelID)
	if err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return stringPtr(sess.ID), nil
}

func sessionDurationSeconds(sess *persistence.Session) int64 {
	start := sess.ProposedAt
	if sess.AcceptedAt != nil {
		start = sess.AcceptedAt.UTC()
	}
	end := time.Now().UTC()
	if sess.ClosedAt != nil {
		end = sess.ClosedAt.UTC()
	}
	if end.Before(start) {
		return 0
	}
	return int64(end.Sub(start).Seconds())
}

func sessionViolationDimension(reason persistence.SessionCloseReason, detail string) string {
	if reason != persistence.SessionCloseReasonContractViolation {
		return ""
	}
	normalized := strings.ToLower(detail)
	switch {
	case strings.Contains(normalized, "max_duration"):
		return "max_duration"
	case strings.Contains(normalized, "envelope_count"):
		return "envelope_count"
	case strings.Contains(normalized, "envelope_bytes"):
		return "envelope_bytes"
	case strings.Contains(normalized, "message_type"):
		return "message_type"
	default:
		return ""
	}
}

func stringPtr(v string) *string {
	return &v
}
