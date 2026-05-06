package agentutil

import (
	"context"
	"errors"
	"time"

	"github.com/openziti/agora/sdk/agent/session"
)

// HoldPastDurationCap keeps an active session open for the caller's
// selected overrun window. The controller reaper owns the actual close.
func HoldPastDurationCap(ctx context.Context, _ *session.Session, hold time.Duration) error {
	if hold <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(hold)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func SendOversizeEnvelope(ctx context.Context, sess *session.Session, messageType string, payloadBytes int) error {
	if payloadBytes < 1 {
		payloadBytes = 1
	}
	payload := make([]byte, payloadBytes)
	for i := range payload {
		payload[i] = 'x'
	}
	return sess.Send(ctx, session.Envelope{
		MessageType: messageType,
		ContentType: "application/octet-stream",
		Payload:     payload,
	})
}

func SendDisallowedMessageType(ctx context.Context, sess *session.Session, messageType string) error {
	return sess.Send(ctx, session.Envelope{
		MessageType: messageType,
		ContentType: "application/json",
		Payload:     []byte(`{}`),
	})
}

func IsContractViolation(err error) bool {
	var violation *session.ErrContractViolation
	return errors.As(err, &violation)
}
