package session

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync/atomic"
	"time"

	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/internal/persistence"
)

// ErrNoTransport is returned when Send/Receive are called on a Session
// that has no transport attached (e.g. sessions-slice-only callers).
var ErrNoTransport = errors.New("session: no transport attached (envelopes slice not wired)")

// ErrContractViolation wraps a runtime-side contract enforcement
// rejection. Close* context carries the specific dimension.
type ErrContractViolation struct {
	Detail string
}

func (e *ErrContractViolation) Error() string { return "contract violation: " + e.Detail }

// Send serializes one envelope to the session's transport, after
// enforcing contract bounds. Fields the session knows about are filled
// in automatically when the caller leaves them unset.
func (s *Session) Send(ctx context.Context, env Envelope) error {
	if s.conn == nil {
		return ErrNoTransport
	}
	// auto-fill
	if env.SchemaVersion == 0 {
		env.SchemaVersion = 1
	}
	if env.EnvelopeID == "" {
		env.EnvelopeID = "evp_" + newRandomSuffix()
	}
	env.SessionID = s.ID
	env.SenderAccountID = s.agent.AccountID()
	if s.agent.Environment() != nil {
		// The agent currently exposes account/org through env metadata;
		// sender_organization_id can be unset if not wired through the
		// agent. Callers may override.
	}
	if env.SenderAccountID == "" {
		env.SenderAccountID = s.ProviderAccountID // provider side default
	}
	if env.SenderOrganizationID == "" {
		env.SenderOrganizationID = s.ProviderOrganizationID
	}
	if env.Timestamp.IsZero() {
		env.Timestamp = time.Now().UTC()
	}

	// enforce outbound
	if s.ContractSnapshot != nil {
		snap := s.ContractSnapshot
		if !messageTypeAllowed(env.MessageType, snap.AllowedMessageTypes) {
			return s.violate(ctx, "message_type_disallowed")
		}
		if snap.MaxEnvelopeCount > 0 {
			if atomic.LoadInt64(&s.sentCount) >= int64(snap.MaxEnvelopeCount) {
				return s.violate(ctx, "envelope_count_exceeded")
			}
		}
	}

	s.streamMu.Lock()
	defer s.streamMu.Unlock()
	size, err := EncodeFrame(s.conn, env)
	if err != nil {
		if errors.Is(err, ErrFrameTooLarge) {
			return s.violate(ctx, "envelope_size_exceeded_platform_ceiling")
		}
		return err
	}
	if s.ContractSnapshot != nil && s.ContractSnapshot.MaxEnvelopeBytes > 0 && size > s.ContractSnapshot.MaxEnvelopeBytes {
		return s.violate(ctx, "envelope_size_exceeded")
	}
	atomic.AddInt64(&s.sentCount, 1)
	return nil
}

// Receive blocks until the next envelope arrives. Returns io.EOF when
// the peer cleanly closes. Enforces inbound contract bounds; violations
// close the session.
func (s *Session) Receive(ctx context.Context) (Envelope, error) {
	if s.conn == nil {
		return Envelope{}, ErrNoTransport
	}
	// respect ctx cancellation by setting a read deadline
	if deadline, ok := ctx.Deadline(); ok {
		_ = s.conn.SetReadDeadline(deadline)
	}
	env, size, err := DecodeFrame(s.conn)
	if err != nil {
		if errors.Is(err, ErrUnknownFrameVersion) {
			_ = s.violate(ctx, "unknown_frame_version")
			return Envelope{}, err
		}
		if errors.Is(err, ErrFrameTooLarge) {
			_ = s.violate(ctx, "envelope_size_exceeded_platform_ceiling")
			return Envelope{}, err
		}
		if errors.Is(err, ErrMalformedHeader) {
			_ = s.violate(ctx, "malformed_envelope_header")
			return Envelope{}, err
		}
		if errors.Is(err, io.EOF) {
			return Envelope{}, io.EOF
		}
		return Envelope{}, err
	}
	if s.ContractSnapshot != nil {
		snap := s.ContractSnapshot
		if snap.MaxEnvelopeBytes > 0 && size > snap.MaxEnvelopeBytes {
			_ = s.violate(ctx, "envelope_size_exceeded")
			return Envelope{}, fmt.Errorf("inbound envelope size %d > max %d", size, snap.MaxEnvelopeBytes)
		}
		if !messageTypeAllowed(env.MessageType, snap.AllowedMessageTypes) {
			_ = s.violate(ctx, "message_type_disallowed")
			return Envelope{}, fmt.Errorf("inbound message_type %q not allowed", env.MessageType)
		}
	}
	atomic.AddInt64(&s.recvCount, 1)
	return env, nil
}

func (s *Session) violate(ctx context.Context, detail string) error {
	body := api.OptCloseSessionRequest{}
	body.SetTo(api.CloseSessionRequest{Reason: api.NewOptString(detail)})
	// best-effort; server may have already closed for another reason
	_, _ = s.agent.Controller().CloseSession(ctx, body, api.CloseSessionParams{SessionId: s.ID})
	teardownTransport(s)
	return &ErrContractViolation{Detail: detail}
}

// messageTypeAllowed applies the contract's allowed_message_types
// filter. Per contracts.md:
//   - empty list  -> "no types allowed" (defensive default)
//   - ["*"]       -> wildcard
//   - otherwise   -> exact match on the list
func messageTypeAllowed(messageType string, allowed []string) bool {
	if len(allowed) == 0 {
		return false
	}
	for _, a := range allowed {
		if a == "*" || a == messageType {
			return true
		}
	}
	return false
}

// newRandomSuffix returns a 12-character lowercase alphanumeric string,
// matching the resource-id format used by the controller.
func newRandomSuffix() string {
	full := persistence.NewResourceID("")
	return full[:12]
}
