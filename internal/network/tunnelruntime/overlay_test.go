package tunnelruntime

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/sdk-golang/ziti"
	"github.com/openziti/sdk-golang/ziti/edge"
)

func TestOpenZitiContextReplacesStaleServiceCache(t *testing.T) {
	first := &fakeZitiContext{}
	replacement := &fakeZitiContext{service: testService("tt_test")}
	created := 0
	overlay := newOpenZitiContext("identity.json", first, func(identityPath string) (zitiContext, error) {
		if identityPath != "identity.json" {
			t.Fatalf("unexpected identity path %q", identityPath)
		}
		created++
		return replacement, nil
	})

	got, generation, err := overlay.contextForService("tt_test")
	if err != nil {
		t.Fatalf("contextForService: %v", err)
	}
	if got != replacement {
		t.Fatalf("expected replacement context, got %#v", got)
	}
	if generation != 1 {
		t.Fatalf("expected replacement generation 1, got %d", generation)
	}
	if created != 1 {
		t.Fatalf("expected one replacement, got %d", created)
	}
	if !first.isClosed() {
		t.Fatal("expected stale context to be closed")
	}
	if replacement.refreshes != 1 {
		t.Fatalf("expected replacement service refresh, got %d", replacement.refreshes)
	}
}

func TestOpenZitiContextReportsUnrecoverableServiceRefresh(t *testing.T) {
	first := &fakeZitiContext{}
	replacement := &fakeZitiContext{}
	overlay := newOpenZitiContext("identity.json", first, func(string) (zitiContext, error) {
		return replacement, nil
	})

	_, _, err := overlay.contextForService("tt_missing")
	if err == nil {
		t.Fatal("expected service refresh failure")
	}
	select {
	case failure := <-overlay.Failures():
		if !errors.Is(failure, err) && failure.Error() != err.Error() {
			t.Fatalf("unexpected reported failure %v", failure)
		}
	case <-time.After(time.Second):
		t.Fatal("expected service refresh failure to be reported")
	}
}

func TestWatchHostingConnectionsReportsPersistentLoss(t *testing.T) {
	listener := &fakeHostingListener{}
	failures, stop := watchHostingConnections(context.Background(), listener, "tt_test", 15*time.Millisecond)
	defer stop()

	listener.emit(nil)
	select {
	case err := <-failures:
		if err == nil {
			t.Fatal("expected hosting failure")
		}
	case <-time.After(time.Second):
		t.Fatal("expected persistent hosting connection loss to fail")
	}
}

func TestWatchHostingConnectionsAllowsRecoveryWithinGracePeriod(t *testing.T) {
	listener := &fakeHostingListener{}
	failures, stop := watchHostingConnections(context.Background(), listener, "tt_test", 50*time.Millisecond)
	defer stop()

	listener.emit(nil)
	time.Sleep(10 * time.Millisecond)
	listener.emit([]edge.RouterHostConn{nil})
	select {
	case err := <-failures:
		t.Fatalf("unexpected hosting failure after recovery: %v", err)
	case <-time.After(75 * time.Millisecond):
	}
}

func TestOverlayFailureStopsRuntimeAndClosesContext(t *testing.T) {
	failure := errors.New("overlay session did not recover")
	overlay := &fakeLifecycleOverlay{failures: make(chan error, 1)}
	listener := &fakeRuntimeCloser{closed: make(chan struct{})}
	handle := newOverlayHandle(context.Background(), overlay, listener, "tt_test", false, func() error {
		<-listener.closed
		return errors.New("listener closed")
	})

	overlay.failures <- failure
	select {
	case err := <-handle.Done():
		if !errors.Is(err, failure) {
			t.Fatalf("expected overlay failure, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("expected runtime to stop after overlay failure")
	}
	if !overlay.isClosed() {
		t.Fatal("expected overlay context to close with runtime")
	}
}

func TestDialContextCancellationClosesLateConnection(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	closed := make(chan struct{})
	conn := &closeNotifyConn{closed: closed}

	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan struct {
		conn net.Conn
		err  error
	}, 1)
	go func() {
		got, err := dialContext(ctx, func() (net.Conn, error) {
			close(started)
			<-release
			return conn, nil
		})
		result <- struct {
			conn net.Conn
			err  error
		}{conn: got, err: err}
	}()

	<-started
	cancel()

	select {
	case res := <-result:
		if !errors.Is(res.err, context.Canceled) {
			t.Fatalf("expected context canceled, got %v", res.err)
		}
		if res.conn != nil {
			t.Fatalf("expected no connection after cancellation, got %#v", res.conn)
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("dialContext did not return promptly on cancellation")
	}

	close(release)
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("late connection was not closed")
	}
}

type closeNotifyConn struct {
	closed chan<- struct{}
}

func (*closeNotifyConn) Read([]byte) (int, error)         { return 0, errors.New("closed") }
func (*closeNotifyConn) Write([]byte) (int, error)        { return 0, errors.New("closed") }
func (c *closeNotifyConn) Close() error                   { close(c.closed); return nil }
func (*closeNotifyConn) LocalAddr() net.Addr              { return testAddr("local") }
func (*closeNotifyConn) RemoteAddr() net.Addr             { return testAddr("remote") }
func (*closeNotifyConn) SetDeadline(time.Time) error      { return nil }
func (*closeNotifyConn) SetReadDeadline(time.Time) error  { return nil }
func (*closeNotifyConn) SetWriteDeadline(time.Time) error { return nil }

type testAddr string

func (a testAddr) Network() string { return string(a) }
func (a testAddr) String() string  { return string(a) }

type fakeZitiContext struct {
	mu         sync.Mutex
	service    *rest_model.ServiceDetail
	refreshErr error
	refreshes  int
	closed     bool
}

func (f *fakeZitiContext) RefreshService(string) (*rest_model.ServiceDetail, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.refreshes++
	return f.service, f.refreshErr
}

func (*fakeZitiContext) ListenWithOptions(string, *ziti.ListenOptions) (edge.Listener, error) {
	return nil, errors.New("unexpected listen")
}

func (*fakeZitiContext) DialContextWithOptions(context.Context, string, *ziti.DialOptions) (edge.Conn, error) {
	return nil, errors.New("unexpected dial")
}

func (f *fakeZitiContext) Close() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
}

func (f *fakeZitiContext) isClosed() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.closed
}

func testService(name string) *rest_model.ServiceDetail {
	id := name
	return &rest_model.ServiceDetail{BaseEntity: rest_model.BaseEntity{ID: &id}, Name: &name}
}

type fakeHostingListener struct {
	mu      sync.Mutex
	handler func([]edge.RouterHostConn)
}

type fakeLifecycleOverlay struct {
	mu       sync.Mutex
	failures chan error
	closed   bool
}

func (*fakeLifecycleOverlay) Listen(string) (net.Listener, error) {
	return nil, errors.New("unexpected listen")
}

func (*fakeLifecycleOverlay) Dial(string) (net.Conn, error) {
	return nil, errors.New("unexpected dial")
}

func (*fakeLifecycleOverlay) DialContext(context.Context, string) (net.Conn, error) {
	return nil, errors.New("unexpected dial")
}

func (f *fakeLifecycleOverlay) Failures() <-chan error { return f.failures }

func (f *fakeLifecycleOverlay) Close() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
}

func (f *fakeLifecycleOverlay) isClosed() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.closed
}

type fakeRuntimeCloser struct {
	once   sync.Once
	closed chan struct{}
}

func (f *fakeRuntimeCloser) Close() error {
	f.once.Do(func() {
		close(f.closed)
	})
	return nil
}

func (f *fakeHostingListener) Close() error { return nil }

func (f *fakeHostingListener) SetConnectionChangeHandler(handler func([]edge.RouterHostConn)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.handler = handler
}

func (f *fakeHostingListener) emit(connections []edge.RouterHostConn) {
	f.mu.Lock()
	handler := f.handler
	f.mu.Unlock()
	if handler != nil {
		handler(connections)
	}
}
