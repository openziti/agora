// Package session provides consumer- and provider-side helpers for
// driving Layer 2 session lifecycle through the Agora controller.
//
// This slice implements the lifecycle-governance half of sessions:
// propose, accept, reject, close, and the provider-side reconciliation
// loop. Runtime-level tunnel attach and byte-level session I/O are
// deferred to the envelopes slice.
package session

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/sdk/agent"
)

const defaultProposeTimeout = 30 * time.Second
const pollInterval = time.Second

type closeSessionController interface {
	CloseSession(context.Context, api.OptCloseSessionRequest, api.CloseSessionParams) (api.CloseSessionRes, error)
}

// Session is the handle returned once a proposal has reached the
// `active` state. Close terminates the session on the controller.
type Session struct {
	ID                     string
	TunnelID               string
	TunnelMode             string
	AdvertisementID        string
	WorkgroupID            string
	ProviderAccountID      string
	ConsumerAccountID      string
	ProviderOrganizationID string
	ConsumerOrganizationID string
	// ContractSnapshot is the frozen contract the controller evaluated
	// against at accept time. Populated when the advertisement has a
	// contract attached; nil otherwise.
	ContractSnapshot *api.ContractSnapshot

	agent                  *agent.Agent
	closeSessionController closeSessionController

	// transport state — populated when the envelopes-slice transport
	// is wired up (consumer side after Propose, provider side in
	// handleProposal). The zero-value Session (sessions slice only)
	// has no transport and cannot Send/Receive.
	conn               net.Conn
	providerListener   net.Listener
	localListenAddress string
	backendAddress     string
	streamMu           sync.Mutex // serializes writes on conn

	sentCount int64 // atomic
	recvCount int64 // atomic
}

// Close closes the session via the controller. The reason is stored as
// close_detail on the terminal session row. Idempotent on already-closed
// sessions. Tears down any attached transport.
func (s *Session) Close(ctx context.Context, reason string) error {
	teardownTransport(s)
	body := api.OptCloseSessionRequest{}
	if reason != "" {
		body.SetTo(api.CloseSessionRequest{Detail: api.NewOptString(reason)})
	}
	controller := s.controllerClient()
	if controller == nil {
		return errors.New("close session: controller client is not configured")
	}
	res, err := controller.CloseSession(ctx, body, api.CloseSessionParams{SessionId: s.ID})
	if err != nil {
		return err
	}
	switch typed := res.(type) {
	case *api.CloseSessionNoContent:
		return nil
	case *api.CloseSessionBadRequest:
		return fmt.Errorf("close session: %s", typed.Message)
	case *api.CloseSessionForbidden:
		return fmt.Errorf("close session: %s", typed.Message)
	case *api.CloseSessionNotFound:
		return fmt.Errorf("close session: %s", typed.Message)
	case *api.CloseSessionUnauthorized:
		return fmt.Errorf("close session: %s", typed.Message)
	case *api.CloseSessionInternalServerError:
		return fmt.Errorf("close session: %s", typed.Message)
	default:
		return fmt.Errorf("close session: unexpected response %T", res)
	}
}

// ProposeOptions configures a consumer-side propose call.
type ProposeOptions struct {
	WorkgroupID string
	Message     string
	Timeout     time.Duration
}

// Proposal is the view of a proposed session handed to a Handler.
type Proposal struct {
	SessionID       string
	ConsumerAccount string
	WorkgroupID     string
	Message         string
}

// Handler receives proposed sessions routed to a registered
// advertisement, decides whether to accept, and optionally runs for the
// lifetime of an active session.
type Handler interface {
	Accept(ctx context.Context, proposal Proposal) error
	Serve(ctx context.Context, sess *Session) error
}

// Propose creates a session against the given advertisement and polls
// for the controller to transition it to `active`. Returns a Session
// handle on success, or an error carrying the close_reason if the
// session closes before reaching active (or the timeout elapses).
func Propose(ctx context.Context, a *agent.Agent, advertisementID string, opts ProposeOptions) (*Session, error) {
	if opts.WorkgroupID == "" {
		return nil, errors.New("session.Propose: WorkgroupID is required")
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = defaultProposeTimeout
	}

	req := &api.ProposeSessionRequest{
		AdvertisementId: advertisementID,
		WorkgroupId:     opts.WorkgroupID,
	}
	if opts.Message != "" {
		req.ProposerMessage.SetTo(opts.Message)
	}

	res, err := a.Controller().ProposeSession(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("propose session: %w", err)
	}
	proposed, ok := res.(*api.Session)
	if !ok {
		return nil, fmt.Errorf("propose session: unexpected response %T: %v", res, describeError(res))
	}

	deadline := time.Now().Add(timeout)
	for {
		current, err := fetchSession(ctx, a, proposed.ID)
		if err != nil {
			return nil, err
		}
		switch current.State {
		case api.SessionStateActive:
			sess := sessionFromAPI(a, current)
			if err := attachConsumerStream(ctx, a, sess); err != nil {
				// attach failure: close the session so the provider unblocks.
				_ = sess.Close(ctx, "consumer_attach_failed")
				return nil, fmt.Errorf("attach transport: %w", err)
			}
			return sess, nil
		case api.SessionStateClosed:
			reason := "<unknown>"
			if current.CloseReason.Set {
				reason = string(current.CloseReason.Value)
			}
			return nil, fmt.Errorf("session closed before active: close_reason='%s'", reason)
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("propose session timed out waiting for active state (session_id='%s' state='%s')", current.ID, current.State)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(pollInterval):
		}
	}
}

// RegisterHandler runs a provider-side reconciliation loop until ctx is
// cancelled. It polls for proposed sessions scoped to advertisementID
// and routes each through the supplied handler.
func RegisterHandler(ctx context.Context, a *agent.Agent, advertisementID string, handler Handler) error {
	if handler == nil {
		return errors.New("session.RegisterHandler: handler is required")
	}
	if advertisementID == "" {
		return errors.New("session.RegisterHandler: advertisementID is required")
	}

	envID := a.Environment().EnvironmentID
	seen := make(map[string]struct{})

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}

		proposed, err := listProposed(ctx, a, advertisementID)
		if err != nil {
			a.Log().With("advertisement_id", advertisementID).Warnf("session handler poll failed: %v", err)
			continue
		}

		for _, s := range proposed {
			if _, ok := seen[s.ID]; ok {
				continue
			}
			seen[s.ID] = struct{}{}
			go handleProposal(ctx, a, envID, s, handler)
		}
	}
}

func handleProposal(ctx context.Context, a *agent.Agent, envID string, s api.Session, handler Handler) {
	proposal := Proposal{
		SessionID:       s.ID,
		ConsumerAccount: s.ConsumerAccountId,
		WorkgroupID:     s.WorkgroupId,
	}
	if s.ProposerMessage.Set {
		proposal.Message = s.ProposerMessage.Value
	}

	if acceptErr := handler.Accept(ctx, proposal); acceptErr != nil {
		rejectReason := acceptErr.Error()
		body := api.OptCloseSessionRequest{}
		body.SetTo(api.CloseSessionRequest{Detail: api.NewOptString(rejectReason)})
		if _, err := a.Controller().RejectSession(ctx, body, api.RejectSessionParams{SessionId: s.ID}); err != nil {
			a.Log().With("session_id", s.ID).Warnf("reject session failed: %v", err)
		}
		return
	}

	// Allocate provider-side listener before accept so we can commit the
	// backend address to the controller atomically with acceptance.
	listener, err := net.Listen("tcp", providerListenAddr+":0")
	if err != nil {
		a.Log().With("session_id", s.ID).Warnf("allocate provider listener: %v", err)
		return
	}
	backendAddr := listener.Addr().String()

	acceptRes, err := a.Controller().AcceptSession(ctx,
		&api.AcceptSessionRequest{
			EnvironmentId:  envID,
			BackendAddress: api.NewOptString(backendAddr),
		},
		api.AcceptSessionParams{SessionId: s.ID})
	if err != nil {
		_ = listener.Close()
		a.Log().With("session_id", s.ID).Warnf("accept session api error: %v", err)
		return
	}
	active, ok := acceptRes.(*api.Session)
	if !ok {
		_ = listener.Close()
		a.Log().With("session_id", s.ID).Warnf("accept session unexpected response: %T %s", acceptRes, describeError(acceptRes))
		return
	}
	sess := sessionFromAPI(a, active)
	if err := attachProviderStream(ctx, a, sess, backendAddr, listener); err != nil {
		a.Log().With("session_id", sess.ID).Warnf("attach provider stream failed: %v", err)
		_ = sess.Close(ctx, "provider_attach_failed")
		return
	}

	reporterCtx, cancelReporter := context.WithCancel(ctx)
	defer cancelReporter()
	go countReporterLoop(reporterCtx, a, sess)

	if serveErr := handler.Serve(ctx, sess); serveErr != nil {
		if closeErr := sess.Close(ctx, serveErr.Error()); closeErr != nil {
			a.Log().With("session_id", sess.ID).Warnf("close after serve error failed: %v", closeErr)
		}
	}
}

func fetchSession(ctx context.Context, a *agent.Agent, id string) (*api.Session, error) {
	res, err := a.Controller().GetSession(ctx, api.GetSessionParams{SessionId: id})
	if err != nil {
		return nil, err
	}
	sess, ok := res.(*api.Session)
	if !ok {
		return nil, fmt.Errorf("get session: unexpected response %T %s", res, describeError(res))
	}
	return sess, nil
}

func listProposed(ctx context.Context, a *agent.Agent, advertisementID string) ([]api.Session, error) {
	params := api.ListSessionsParams{
		State:           []api.SessionState{api.SessionStateProposed},
		Role:            api.NewOptListSessionsRole(api.ListSessionsRoleProvider),
		AdvertisementId: api.NewOptString(advertisementID),
	}
	res, err := a.Controller().ListSessions(ctx, params)
	if err != nil {
		return nil, err
	}
	listing, ok := res.(*api.ListSessionsResponse)
	if !ok {
		return nil, fmt.Errorf("list sessions: unexpected response %T %s", res, describeError(res))
	}
	return *listing, nil
}

func sessionFromAPI(a *agent.Agent, s *api.Session) *Session {
	out := &Session{
		ID:                     s.ID,
		TunnelMode:             string(s.TunnelMode),
		AdvertisementID:        s.AdvertisementId,
		WorkgroupID:            s.WorkgroupId,
		ProviderAccountID:      s.ProviderAccountId,
		ConsumerAccountID:      s.ConsumerAccountId,
		ProviderOrganizationID: s.ProviderOrganizationId,
		ConsumerOrganizationID: s.ConsumerOrganizationId,
		agent:                  a,
	}
	if s.TunnelId.Set {
		out.TunnelID = s.TunnelId.Value
	}
	if s.ContractSnapshot.Set {
		snap := s.ContractSnapshot.Value
		out.ContractSnapshot = &snap
	}
	return out
}

func (s *Session) controllerClient() closeSessionController {
	if s.closeSessionController != nil {
		return s.closeSessionController
	}
	if s.agent == nil {
		return nil
	}
	return s.agent.Controller()
}

// describeError extracts a short message from generated error responses.
// Generated error types are aliases for api.Error; reinterpret via the
// underlying layout when possible.
func describeError(res any) string {
	switch v := res.(type) {
	case *api.Error:
		return v.Message
	}
	// Generated error types are `type Foo api.Error`, which means direct
	// field access works through a pointer conversion. We can't do a
	// single cast, so just return the go type name; callers also log %T.
	return fmt.Sprintf("%T", res)
}
