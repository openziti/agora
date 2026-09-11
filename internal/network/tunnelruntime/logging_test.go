package tunnelruntime

import (
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/openziti/sdk-golang/ziti/edge"
)

func TestHTTPRequestLoggerFlushesStreamingResponseHeaders(t *testing.T) {
	releaseOrigin := make(chan struct{})
	var releaseOnce sync.Once
	release := func() {
		releaseOnce.Do(func() {
			close(releaseOrigin)
		})
	}
	t.Cleanup(release)

	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		select {
		case <-releaseOrigin:
		case <-r.Context().Done():
		}
	}))
	defer origin.Close()

	target, err := url.Parse(origin.URL)
	if err != nil {
		t.Fatalf("parse origin URL: %v", err)
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	tunnel := httptest.NewServer(connectHTTPRequestLogger("stream-test", "127.0.0.1:0", proxy))
	defer tunnel.Close()

	type responseResult struct {
		response *http.Response
		err      error
	}
	responseCh := make(chan responseResult, 1)
	go func() {
		response, err := tunnel.Client().Get(tunnel.URL + "/stream")
		responseCh <- responseResult{response: response, err: err}
	}()

	select {
	case result := <-responseCh:
		if result.err != nil {
			t.Fatalf("request streaming response: %v", result.err)
		}
		defer result.response.Body.Close()
		if result.response.StatusCode != http.StatusOK {
			t.Fatalf("expected status %d, got %d", http.StatusOK, result.response.StatusCode)
		}
		if contentType := result.response.Header.Get("Content-Type"); contentType != "text/event-stream" {
			t.Fatalf("expected streaming content type, got %q", contentType)
		}
	case <-time.After(time.Second):
		release()
		t.Fatal("streaming response headers were not forwarded before the first body byte")
	}
}

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
