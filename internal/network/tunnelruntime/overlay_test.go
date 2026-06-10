package tunnelruntime

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"
)

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
