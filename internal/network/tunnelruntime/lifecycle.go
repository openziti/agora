package tunnelruntime

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/openziti/sdk-golang/ziti/edge"
)

const overlayListenerRecoveryTimeout = 45 * time.Second

type overlayCloser interface {
	Close()
}

type overlayFailureSource interface {
	Failures() <-chan error
}

func closeOverlay(overlay OverlayContext) {
	if closer, ok := overlay.(overlayCloser); ok {
		closer.Close()
	}
}

func overlayFailures(overlay OverlayContext) <-chan error {
	if source, ok := overlay.(overlayFailureSource); ok {
		return source.Failures()
	}
	return nil
}

func newOverlayHandle(ctx context.Context, overlay OverlayContext, listener io.Closer, serviceName string, watchHosting bool, run func() error) *Handle {
	return newHandle(func() error {
		defer closeOverlay(overlay)
		failures := overlayFailures(overlay)

		var hostingFailures <-chan error
		stopHostingWatch := func() {}
		if watchHosting {
			hostingFailures, stopHostingWatch = watchHostingConnections(ctx, listener, serviceName, overlayListenerRecoveryTimeout)
		}
		defer stopHostingWatch()

		runDone := make(chan error, 1)
		go func() {
			runDone <- run()
		}()

		for {
			select {
			case err := <-runDone:
				select {
				case failure := <-failures:
					return failure
				case failure := <-hostingFailures:
					return failure
				default:
					return err
				}
			case failure := <-failures:
				_ = listener.Close()
				<-runDone
				return failure
			case failure := <-hostingFailures:
				_ = listener.Close()
				<-runDone
				return failure
			}
		}
	})
}

type hostingConnectionListener interface {
	SetConnectionChangeHandler(func([]edge.RouterHostConn))
}

func watchHostingConnections(ctx context.Context, listener io.Closer, serviceName string, timeout time.Duration) (<-chan error, func()) {
	failures := make(chan error, 1)
	sessionListener, ok := listener.(hostingConnectionListener)
	if !ok {
		return nil, func() {}
	}

	var mu sync.Mutex
	var timer *time.Timer
	stopped := false
	done := make(chan struct{})
	var stopOnce sync.Once
	stop := func() {
		stopOnce.Do(func() {
			mu.Lock()
			stopped = true
			if timer != nil {
				timer.Stop()
				timer = nil
			}
			mu.Unlock()
			close(done)
		})
	}

	sessionListener.SetConnectionChangeHandler(func(connections []edge.RouterHostConn) {
		mu.Lock()
		defer mu.Unlock()
		if stopped {
			return
		}
		if len(connections) > 0 {
			if timer != nil {
				timer.Stop()
				timer = nil
			}
			return
		}
		if timer != nil {
			return
		}
		timer = time.AfterFunc(timeout, func() {
			mu.Lock()
			defer mu.Unlock()
			if stopped || timer == nil {
				return
			}
			timer = nil
			select {
			case failures <- fmt.Errorf("overlay listener for service '%s' has no hosting connections after %s", serviceName, timeout):
			default:
			}
		})
	})

	go func() {
		select {
		case <-ctx.Done():
			stop()
		case <-done:
		}
	}()

	return failures, stop
}
