package tunnelruntime

import (
	"net"
	"testing"
	"time"

	"github.com/openziti/sdk-golang/ziti/edge"
)

func TestOverlayPeerInfoPrefersRouterAttestedIdentity(t *testing.T) {
	conn := &fakeServiceConn{
		fakeBaseServiceConn: fakeBaseServiceConn{
			sourceIdentifier: "self-reported",
			circuitID:        "circuit-1",
			appData:          []byte("metadata"),
		},
		dialerIdentityID: "dialer-1",
		dialerName:       "router-attested",
	}

	info := overlayPeerInfoFromConn(conn)

	if info.IdentityName != "router-attested" {
		t.Fatalf("expected router-attested identity name, got %q", info.IdentityName)
	}
	if info.IdentityID != "dialer-1" {
		t.Fatalf("expected router-attested identity id, got %q", info.IdentityID)
	}
	if info.IdentityTrust != "router_attested" {
		t.Fatalf("expected router_attested trust, got %q", info.IdentityTrust)
	}
	if info.SourceIdentifier != "self-reported" {
		t.Fatalf("expected source identifier, got %q", info.SourceIdentifier)
	}
	if info.CircuitID != "circuit-1" {
		t.Fatalf("expected circuit id, got %q", info.CircuitID)
	}
	if info.AppDataLength != len(conn.appData) {
		t.Fatalf("expected app data length %d, got %d", len(conn.appData), info.AppDataLength)
	}
}

func TestOverlayPeerInfoWithoutRouterIdentityLeavesIdentityUnset(t *testing.T) {
	conn := &fakeBaseServiceConn{
		sourceIdentifier: "caller-id",
		circuitID:        "circuit-2",
	}

	info := overlayPeerInfoFromConn(conn)

	if info.IdentityName != "" {
		t.Fatalf("expected no identity name, got %q", info.IdentityName)
	}
	if info.IdentityID != "" {
		t.Fatalf("expected no identity id, got %q", info.IdentityID)
	}
	if info.IdentityTrust != "missing" {
		t.Fatalf("expected missing trust, got %q", info.IdentityTrust)
	}
	if info.SourceIdentifier != "caller-id" {
		t.Fatalf("expected source identifier, got %q", info.SourceIdentifier)
	}
}

type fakeBaseServiceConn struct {
	sourceIdentifier string
	circuitID        string
	appData          []byte
}

type fakeServiceConn struct {
	fakeBaseServiceConn
	dialerIdentityID string
	dialerName       string
}

func (f *fakeBaseServiceConn) Read([]byte) (int, error)         { return 0, nil }
func (f *fakeBaseServiceConn) Write([]byte) (int, error)        { return 0, nil }
func (f *fakeBaseServiceConn) Close() error                     { return nil }
func (f *fakeBaseServiceConn) LocalAddr() net.Addr              { return fakeAddr("local") }
func (f *fakeBaseServiceConn) RemoteAddr() net.Addr             { return fakeAddr("remote") }
func (f *fakeBaseServiceConn) SetDeadline(time.Time) error      { return nil }
func (f *fakeBaseServiceConn) SetReadDeadline(time.Time) error  { return nil }
func (f *fakeBaseServiceConn) SetWriteDeadline(time.Time) error { return nil }
func (f *fakeBaseServiceConn) CloseWrite() error                { return nil }
func (f *fakeBaseServiceConn) IsClosed() bool                   { return false }
func (f *fakeBaseServiceConn) GetAppData() []byte               { return f.appData }
func (f *fakeBaseServiceConn) SourceIdentifier() string         { return f.sourceIdentifier }
func (f *fakeBaseServiceConn) TraceRoute(uint32, time.Duration) (*edge.TraceRouteResult, error) {
	return nil, nil
}
func (f *fakeBaseServiceConn) GetCircuitId() string       { return f.circuitID }
func (f *fakeBaseServiceConn) GetStickinessToken() []byte { return nil }
func (f *fakeBaseServiceConn) GetDialerIdentityId() string {
	return ""
}
func (f *fakeBaseServiceConn) GetDialerIdentityName() string {
	return ""
}
func (f *fakeServiceConn) GetDialerIdentityId() string   { return f.dialerIdentityID }
func (f *fakeServiceConn) GetDialerIdentityName() string { return f.dialerName }

type fakeAddr string

func (a fakeAddr) Network() string { return "ziti" }
func (a fakeAddr) String() string  { return string(a) }
