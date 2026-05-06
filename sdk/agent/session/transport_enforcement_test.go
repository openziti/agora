package session

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/openziti/agora/internal/api"
)

func TestSendOversizedEnvelopeReportsContractViolation(t *testing.T) {
	t.Parallel()

	controller := &fakeCloseSessionController{}
	sess := &Session{
		ID:                     "ses_aaaaaaaaaaaa",
		ProviderAccountID:      "ac_provideraaaa",
		ProviderOrganizationID: "org_provideraaa",
		ContractSnapshot: &api.ContractSnapshot{
			MaxEnvelopeBytes:    1,
			AllowedMessageTypes: []string{"markets.equity.request"},
		},
		conn:                   &bufferConn{},
		closeSessionController: controller,
	}

	err := sess.Send(context.Background(), Envelope{
		MessageType: "markets.equity.request",
		Payload:     []byte("payload"),
	})
	var violation *ErrContractViolation
	if !errors.As(err, &violation) {
		t.Fatalf("expected ErrContractViolation, got %v", err)
	}
	if !strings.Contains(violation.Detail, "outbound envelope size") || !strings.Contains(violation.Detail, "exceeds contract max 1") {
		t.Fatalf("unexpected violation detail %q", violation.Detail)
	}
	if len(controller.requests) != 1 {
		t.Fatalf("expected 1 close request, got %d", len(controller.requests))
	}

	req := controller.requests[0]
	if !req.Set {
		t.Fatalf("expected close request body to be set")
	}
	if !req.Value.Reason.Set || req.Value.Reason.Value != api.SessionCloseReasonContractViolation {
		t.Fatalf("expected contract_violation reason, got %+v", req.Value.Reason)
	}
	if !req.Value.Detail.Set || req.Value.Detail.Value != violation.Detail {
		t.Fatalf("expected violation detail on close request, got %+v", req.Value.Detail)
	}
	if len(controller.params) != 1 || controller.params[0].SessionId != sess.ID {
		t.Fatalf("unexpected close params %+v", controller.params)
	}
}

type fakeCloseSessionController struct {
	requests []api.OptCloseSessionRequest
	params   []api.CloseSessionParams
}

func (f *fakeCloseSessionController) CloseSession(_ context.Context, req api.OptCloseSessionRequest, params api.CloseSessionParams) (api.CloseSessionRes, error) {
	f.requests = append(f.requests, req)
	f.params = append(f.params, params)
	return &api.CloseSessionNoContent{}, nil
}

type bufferConn struct {
	bytes.Buffer
	closed bool
}

func (c *bufferConn) Read([]byte) (int, error) {
	return 0, io.EOF
}

func (c *bufferConn) Write(p []byte) (int, error) {
	if c.closed {
		return 0, net.ErrClosed
	}
	return c.Buffer.Write(p)
}

func (c *bufferConn) Close() error {
	c.closed = true
	return nil
}

func (c *bufferConn) LocalAddr() net.Addr {
	return testAddr("local")
}

func (c *bufferConn) RemoteAddr() net.Addr {
	return testAddr("remote")
}

func (c *bufferConn) SetDeadline(time.Time) error {
	return nil
}

func (c *bufferConn) SetReadDeadline(time.Time) error {
	return nil
}

func (c *bufferConn) SetWriteDeadline(time.Time) error {
	return nil
}

type testAddr string

func (a testAddr) Network() string {
	return "test"
}

func (a testAddr) String() string {
	return string(a)
}
