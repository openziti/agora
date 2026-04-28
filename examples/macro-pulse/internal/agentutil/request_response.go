package agentutil

import (
	"context"
	"encoding/json"
	"errors"
	"io"

	"github.com/openziti/agora/sdk/agent"
	"github.com/openziti/agora/sdk/agent/session"
)

// RequestResponseHandler reads one request envelope from each session,
// decodes the JSON payload into Req, calls the supplied function, and
// writes the result back as a single response envelope (with the
// message_type's `.request` suffix flipped to `.response`).
//
// Caller provides:
//   - agent (for logging),
//   - the responseMessageType to use on the outbound envelope,
//   - the handler function that receives the typed request and
//     returns the typed response.
type RequestResponseHandler[Req any, Resp any] struct {
	Agent               *agent.Agent
	ResponseMessageType string
	ContentType         string
	Handle              func(ctx context.Context, req Req) (Resp, error)
}

func (h *RequestResponseHandler[Req, Resp]) Accept(_ context.Context, proposal session.Proposal) error {
	h.Agent.Log().
		With("session_id", proposal.SessionID).
		With("consumer_account", proposal.ConsumerAccount).
		With("workgroup_id", proposal.WorkgroupID).
		Infof("accepting session proposal")
	return nil
}

func (h *RequestResponseHandler[Req, Resp]) Serve(ctx context.Context, sess *session.Session) error {
	in, err := sess.Receive(ctx)
	if err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return err
	}

	var req Req
	if len(in.Payload) > 0 {
		if err := json.Unmarshal(in.Payload, &req); err != nil {
			h.Agent.Log().With("session_id", sess.ID).With("envelope_id", in.EnvelopeID).Warnf("decode request: %v", err)
			return err
		}
	}
	resp, err := h.Handle(ctx, req)
	if err != nil {
		return err
	}
	respBytes, err := json.Marshal(resp)
	if err != nil {
		return err
	}

	out := session.Envelope{
		MessageType:   h.ResponseMessageType,
		ContentType:   h.contentType(),
		CorrelationID: in.EnvelopeID,
		Payload:       respBytes,
	}
	if err := sess.Send(ctx, out); err != nil {
		return err
	}
	h.Agent.Log().
		With("session_id", sess.ID).
		With("request_envelope_id", in.EnvelopeID).
		With("response_message_type", out.MessageType).
		With("response_bytes", len(respBytes)).
		Infof("served request")

	// Wait for the consumer to close (Receive returns io.EOF). Without
	// this the provider returning here would race with the consumer's
	// Receive of our response.
	_, _ = sess.Receive(ctx)
	return nil
}

func (h *RequestResponseHandler[Req, Resp]) contentType() string {
	if h.ContentType != "" {
		return h.ContentType
	}
	return "application/json"
}
