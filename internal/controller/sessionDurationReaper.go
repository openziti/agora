package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/michaelquigley/df/dl"
	"github.com/openziti/agora/internal/persistence"
)

const sessionDurationReapInterval = 15 * time.Second

// RunSessionDurationReaper loops until ctx is done, periodically
// closing active sessions whose contract snapshot's max_duration_seconds
// has elapsed since accept.
func (s *Service) RunSessionDurationReaper(ctx context.Context) {
	ticker := time.NewTicker(sessionDurationReapInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			if err := s.ReapExpiredSessions(ctx, now.UTC()); err != nil {
				dl.Errorf("session duration reaper failed: %v", err)
			}
		}
	}
}

// ReapExpiredSessions evaluates each active session with a positive
// duration cap on its snapshot and closes it with reason=contract_violation
// if `now - accepted_at >= max_duration_seconds`.
func (s *Service) ReapExpiredSessions(ctx context.Context, now time.Time) error {
	rows, err := s.store.Sessions.ListActiveWithDurationCap(ctx, s.store.DB())
	if err != nil {
		return err
	}
	for i := range rows {
		sess := rows[i]
		if sess.AcceptedAt == nil || len(sess.ContractSnapshotJSON) == 0 {
			continue
		}
		var snap persistence.ContractSnapshot
		if err := json.Unmarshal(sess.ContractSnapshotJSON, &snap); err != nil {
			dl.Warnf("session duration reaper: unable to decode snapshot session_id='%s': %v", sess.ID, err)
			continue
		}
		if snap.MaxDurationSeconds <= 0 {
			continue
		}
		expiresAt := sess.AcceptedAt.Add(time.Duration(snap.MaxDurationSeconds) * time.Second)
		if now.Before(expiresAt) {
			continue
		}
		detail := fmt.Sprintf("max_duration_exceeded: %d seconds", snap.MaxDurationSeconds)
		if err := s.teardownSession(ctx, &sess, persistence.SessionCloseReasonContractViolation, detail); err != nil {
			dl.Warnf("session duration reaper teardown failed session_id='%s': %v", sess.ID, err)
			continue
		}
		dl.Infof("session duration reaper closed session_id='%s' contract_id='%s' max_duration_seconds=%d", sess.ID, snap.ContractID, snap.MaxDurationSeconds)
	}
	return nil
}
