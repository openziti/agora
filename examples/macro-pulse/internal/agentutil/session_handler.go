package agentutil

import (
	"context"

	"github.com/openziti/agora/sdk/agent"
	"github.com/openziti/agora/sdk/agent/session"
)

// LoggingSessionHandler is a Handler implementation useful for the
// macro-pulse demo agents in the sessions slice. It accepts every
// proposal, logs the session lifecycle transitions, and exits Serve
// when the context is cancelled (letting the caller close the session).
//
// The envelopes slice replaces this with a real byte-exchange Handler
// that reads/writes structured envelopes over the backing tunnel.
type LoggingSessionHandler struct {
	Agent *agent.Agent
}

func (h *LoggingSessionHandler) Accept(_ context.Context, proposal session.Proposal) error {
	h.Agent.Log().
		With("session_id", proposal.SessionID).
		With("consumer_account", proposal.ConsumerAccount).
		With("workgroup_id", proposal.WorkgroupID).
		Infof("accepting session proposal")
	return nil
}

func (h *LoggingSessionHandler) Serve(ctx context.Context, sess *session.Session) error {
	h.Agent.Log().
		With("session_id", sess.ID).
		With("tunnel_id", sess.TunnelID).
		With("tunnel_mode", sess.TunnelMode).
		Infof("session active; awaiting close")
	<-ctx.Done()
	return nil
}
