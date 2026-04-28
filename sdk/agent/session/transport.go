package session

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/sdk/agent"
	"github.com/openziti/agora/sdk/agent/networkpb"
)

const (
	envelopeCountReportInterval = 10 * time.Second
	proposeDialTimeout          = 5 * time.Second
	providerListenAddr          = "127.0.0.1"
)

// pickEphemeralLoopbackPort grabs a free ephemeral port on loopback,
// closes the listener, and returns the address as "127.0.0.1:<port>".
// There is a narrow race: another process could claim the port before
// the caller rebinds it. In practice the kernel does not re-assign
// recently-freed ports for a short window, so this is acceptable for
// MVP local-loopback use.
func pickEphemeralLoopbackPort() (string, error) {
	l, err := net.Listen("tcp", providerListenAddr+":0")
	if err != nil {
		return "", err
	}
	addr := l.Addr().String()
	if err := l.Close(); err != nil {
		return "", err
	}
	return addr, nil
}

// attachConsumerStream wires the consumer-side envelope stream once the
// controller reports state=active. It calls EnsureConnect on the
// embedded runtime with a picked ephemeral listen address, then dials
// that address to open a single byte stream for envelope framing.
func attachConsumerStream(ctx context.Context, a *agent.Agent, sess *Session) error {
	rt := a.Runtime()
	if rt == nil {
		return errors.New("attachConsumerStream: agent has no embedded runtime (WithRuntime not set)")
	}
	addr, err := pickEphemeralLoopbackPort()
	if err != nil {
		return fmt.Errorf("pick loopback port: %w", err)
	}
	if _, err := rt.EnsureConnect(ctx, &networkpb.EnsureConnectRequest{
		TunnelId:      sess.TunnelID,
		Name:          "session-" + sess.ID,
		ListenAddress: addr,
	}); err != nil {
		return fmt.Errorf("ensure connect: %w", err)
	}

	dialer := net.Dialer{}
	dialCtx, cancel := context.WithTimeout(ctx, proposeDialTimeout)
	defer cancel()
	conn, err := dialer.DialContext(dialCtx, "tcp", addr)
	if err != nil {
		_, _ = rt.RemoveConnect(ctx, &networkpb.RemoveConnectRequest{
			TunnelId:      sess.TunnelID,
			Name:          "session-" + sess.ID,
			ListenAddress: addr,
		})
		return fmt.Errorf("dial local listener: %w", err)
	}
	sess.conn = conn
	sess.localListenAddress = addr
	return nil
}

// attachProviderStream wires the provider-side envelope stream. It
// allocates a local TCP listener, asks the runtime to bind the ziti
// serve side to that listener's address, and waits for exactly one
// inbound connection (the consumer's dial) before returning.
func attachProviderStream(ctx context.Context, a *agent.Agent, sess *Session, backendAddr string, listener net.Listener) error {
	rt := a.Runtime()
	if rt == nil {
		return errors.New("attachProviderStream: agent has no embedded runtime (WithRuntime not set)")
	}
	if _, err := rt.EnsureServe(ctx, &networkpb.EnsureServeRequest{
		TunnelId:      sess.TunnelID,
		Name:          "session-" + sess.ID,
		Mode:          sess.TunnelMode,
		BackendTarget: backendAddr,
	}); err != nil {
		return fmt.Errorf("ensure serve: %w", err)
	}

	acceptCh := make(chan acceptResult, 1)
	go func() {
		conn, err := listener.Accept()
		acceptCh <- acceptResult{conn, err}
	}()
	select {
	case res := <-acceptCh:
		if res.err != nil {
			return fmt.Errorf("listener accept: %w", res.err)
		}
		sess.conn = res.conn
		sess.providerListener = listener
		sess.backendAddress = backendAddr
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

type acceptResult struct {
	conn net.Conn
	err  error
}

// countReporterLoop runs as a provider-side goroutine, periodically
// reporting the session's envelope count to the controller. Exits when
// ctx is cancelled.
func countReporterLoop(ctx context.Context, a *agent.Agent, sess *Session) {
	ticker := time.NewTicker(envelopeCountReportInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			reportEnvelopeCount(ctx, a, sess)
			return
		case <-ticker.C:
			reportEnvelopeCount(ctx, a, sess)
		}
	}
}

func reportEnvelopeCount(ctx context.Context, a *agent.Agent, sess *Session) {
	total := atomic.LoadInt64(&sess.sentCount) + atomic.LoadInt64(&sess.recvCount)
	if total == 0 {
		return
	}
	_, err := a.Controller().ReportSessionEnvelopeCount(ctx,
		&api.ReportEnvelopeCountRequest{Count: int(total), ObservedAt: time.Now().UTC()},
		api.ReportSessionEnvelopeCountParams{SessionId: sess.ID})
	if err != nil {
		a.Log().With("session_id", sess.ID).Warnf("report envelope count failed: %v", err)
	}
}

// teardownTransport closes the local transport resources (conn +
// listener). Safe to call multiple times.
func teardownTransport(sess *Session) {
	sess.streamMu.Lock()
	defer sess.streamMu.Unlock()
	if sess.conn != nil {
		_ = sess.conn.Close()
		sess.conn = nil
	}
	if sess.providerListener != nil {
		_ = sess.providerListener.Close()
		sess.providerListener = nil
	}
}

var _ sync.Locker = (*sync.Mutex)(nil) // silence unused import in some builds
