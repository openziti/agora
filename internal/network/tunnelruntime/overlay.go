package tunnelruntime

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/michaelquigley/df/dl"
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/sdk-golang/ziti"
	"github.com/openziti/sdk-golang/ziti/edge"
)

type OverlayFactory interface {
	New(identityPath string) (OverlayContext, error)
}

type OverlayContext interface {
	Listen(serviceName string) (net.Listener, error)
	Dial(serviceName string) (net.Conn, error)
	DialContext(ctx context.Context, serviceName string) (net.Conn, error)
}

type OpenZitiFactory struct{}

type zitiContext interface {
	RefreshService(serviceName string) (*rest_model.ServiceDetail, error)
	ListenWithOptions(serviceName string, options *ziti.ListenOptions) (edge.Listener, error)
	DialContextWithOptions(ctx context.Context, serviceName string, options *ziti.DialOptions) (edge.Conn, error)
	Close()
}

type openZitiContext struct {
	mu           sync.RWMutex
	identityPath string
	ctx          zitiContext
	generation   uint64
	closed       bool
	newContext   func(string) (zitiContext, error)
	failures     chan error
	failureOnce  sync.Once
}

func (OpenZitiFactory) New(identityPath string) (OverlayContext, error) {
	ctx, err := loadOpenZitiContext(identityPath)
	if err != nil {
		return nil, err
	}
	return newOpenZitiContext(identityPath, ctx, loadOpenZitiContext), nil
}

func loadOpenZitiContext(identityPath string) (zitiContext, error) {
	cfg, err := ziti.NewConfigFromFile(identityPath)
	if err != nil {
		return nil, err
	}
	ctx, err := ziti.NewContext(cfg)
	if err != nil {
		return nil, err
	}
	return ctx, nil
}

func newOpenZitiContext(identityPath string, ctx zitiContext, newContext func(string) (zitiContext, error)) *openZitiContext {
	return &openZitiContext{
		identityPath: identityPath,
		ctx:          ctx,
		newContext:   newContext,
		failures:     make(chan error, 1),
	}
}

func (c *openZitiContext) Listen(serviceName string) (net.Listener, error) {
	ctx, _, err := c.contextForService(serviceName)
	if err != nil {
		return nil, err
	}
	return ctx.ListenWithOptions(serviceName, &ziti.ListenOptions{
		DoNotSaveDialerIdentity:      false,
		ConnectTimeout:               5 * time.Minute,
		WaitForNEstablishedListeners: 1,
	})
}

func (c *openZitiContext) Dial(serviceName string) (net.Conn, error) {
	return c.DialContext(context.Background(), serviceName)
}

func (c *openZitiContext) DialContext(ctx context.Context, serviceName string) (net.Conn, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	zitiCtx, generation, err := c.contextForService(serviceName)
	if err != nil {
		return nil, err
	}
	conn, err := zitiCtx.DialContextWithOptions(ctx, serviceName, &ziti.DialOptions{ConnectTimeout: 30 * time.Second})
	if err == nil {
		return conn, nil
	}

	// another caller may have replaced a stale context while this dial was in flight.
	// retry once on the replacement rather than returning an error from the retired context.
	current, currentGeneration, currentErr := c.currentContext()
	if currentErr == nil && currentGeneration != generation {
		if refreshErr := refreshService(current, serviceName); refreshErr == nil {
			return current.DialContextWithOptions(ctx, serviceName, &ziti.DialOptions{ConnectTimeout: 30 * time.Second})
		}
	}
	return nil, err
}

func (c *openZitiContext) Close() {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.closed = true
	c.generation++
	ctx := c.ctx
	c.ctx = nil
	c.mu.Unlock()

	if ctx != nil {
		ctx.Close()
	}
}

func (c *openZitiContext) Failures() <-chan error {
	return c.failures
}

func (c *openZitiContext) contextForService(serviceName string) (zitiContext, uint64, error) {
	ctx, generation, err := c.currentContext()
	if err != nil {
		return nil, 0, err
	}
	firstErr := refreshService(ctx, serviceName)
	if firstErr == nil {
		return ctx, generation, nil
	}

	dl.Warnf("rebuilding OpenZiti context for service='%s' after refresh failure: %v", serviceName, firstErr)
	replacement, replacementGeneration, replaceErr := c.replaceContext(generation)
	if replaceErr != nil {
		err = fmt.Errorf("recover overlay context for service '%s': %w", serviceName, errors.Join(firstErr, replaceErr))
		c.reportFailure(err)
		return nil, 0, err
	}
	if err := refreshService(replacement, serviceName); err != nil {
		err = fmt.Errorf("refresh service '%s' after overlay context recovery: %w", serviceName, errors.Join(firstErr, err))
		c.reportFailure(err)
		return nil, 0, err
	}
	return replacement, replacementGeneration, nil
}

func refreshService(ctx zitiContext, serviceName string) error {
	service, err := ctx.RefreshService(serviceName)
	if err != nil {
		return err
	}
	if service == nil {
		return fmt.Errorf("service '%s' not found", serviceName)
	}
	return nil
}

func (c *openZitiContext) currentContext() (zitiContext, uint64, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.closed || c.ctx == nil {
		return nil, 0, errors.New("overlay context is closed")
	}
	return c.ctx, c.generation, nil
}

func (c *openZitiContext) replaceContext(expectedGeneration uint64) (zitiContext, uint64, error) {
	replacement, err := c.newContext(c.identityPath)
	if err != nil {
		return nil, 0, err
	}

	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		replacement.Close()
		return nil, 0, errors.New("overlay context is closed")
	}
	if c.generation != expectedGeneration {
		ctx := c.ctx
		generation := c.generation
		c.mu.Unlock()
		replacement.Close()
		return ctx, generation, nil
	}
	retired := c.ctx
	c.ctx = replacement
	c.generation++
	generation := c.generation
	c.mu.Unlock()

	if retired != nil {
		retired.Close()
	}
	return replacement, generation, nil
}

func (c *openZitiContext) reportFailure(err error) {
	if err == nil {
		return
	}
	c.failureOnce.Do(func() {
		c.failures <- err
	})
}

func dialContext(ctx context.Context, dial func() (net.Conn, error)) (net.Conn, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	type dialResult struct {
		conn net.Conn
		err  error
	}
	result := make(chan dialResult, 1)
	go func() {
		conn, err := dial()
		result <- dialResult{conn: conn, err: err}
	}()

	select {
	case res := <-result:
		return res.conn, res.err
	case <-ctx.Done():
		go func() {
			res := <-result
			if res.conn != nil {
				_ = res.conn.Close()
			}
		}()
		return nil, ctx.Err()
	}
}
