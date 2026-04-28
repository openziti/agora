package agentutil

import (
	"context"
	"errors"
	"io"
	"strings"

	"github.com/openziti/agora/sdk/agent"
	"github.com/openziti/agora/sdk/agent/session"
)

// EchoSessionHandler is a Handler implementation for the macro-pulse
// demo agents. It accepts every proposal, reads a single envelope,
// flips the message_type from `.request` to `.response`, and echoes the
// payload back as an envelope response. After the one ping exchange it
// returns and lets the session close cleanly.
//
// This is the envelopes-slice Handler. The sessions-slice LoggingSessionHandler
// has been replaced with this one because envelope transport is now wired
// through the SDK.
type EchoSessionHandler struct {
	Agent *agent.Agent
}

func (h *EchoSessionHandler) Accept(_ context.Context, proposal session.Proposal) error {
	h.Agent.Log().
		With("session_id", proposal.SessionID).
		With("consumer_account", proposal.ConsumerAccount).
		With("workgroup_id", proposal.WorkgroupID).
		Infof("accepting session proposal")
	return nil
}

func (h *EchoSessionHandler) Serve(ctx context.Context, sess *session.Session) error {
	h.Agent.Log().
		With("session_id", sess.ID).
		With("tunnel_id", sess.TunnelID).
		With("tunnel_mode", sess.TunnelMode).
		Infof("session active; awaiting ping envelope")

	req, err := sess.Receive(ctx)
	if err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return err
	}
	h.Agent.Log().
		With("session_id", sess.ID).
		With("envelope_id", req.EnvelopeID).
		With("message_type", req.MessageType).
		With("payload_bytes", len(req.Payload)).
		Infof("received request; echoing response")

	reply := session.Envelope{
		MessageType:   flipRequestToResponse(req.MessageType),
		ContentType:   req.ContentType,
		CorrelationID: req.EnvelopeID,
		Payload:       req.Payload,
	}
	if err := sess.Send(ctx, reply); err != nil {
		return err
	}
	// Wait for the consumer to close (Receive returns io.EOF). Returning
	// here immediately would cause us to close the session while the
	// consumer is still reading the reply.
	_, _ = sess.Receive(ctx)
	return nil
}

// flipRequestToResponse converts dotted message types like
// "markets.equity.request" → "markets.equity.response". Returns the
// input unchanged if it does not end in ".request".
func flipRequestToResponse(mt string) string {
	const suffix = ".request"
	if strings.HasSuffix(mt, suffix) {
		return strings.TrimSuffix(mt, suffix) + ".response"
	}
	return mt + ".response"
}
