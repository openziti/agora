package tunnelruntime

import (
	"context"
	"net"
	"time"

	"github.com/openziti/sdk-golang/ziti"
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

type openZitiContext struct {
	ctx ziti.Context
}

func (OpenZitiFactory) New(identityPath string) (OverlayContext, error) {
	cfg, err := ziti.NewConfigFromFile(identityPath)
	if err != nil {
		return nil, err
	}
	ctx, err := ziti.NewContext(cfg)
	if err != nil {
		return nil, err
	}
	return &openZitiContext{ctx: ctx}, nil
}

func (c *openZitiContext) Listen(serviceName string) (net.Listener, error) {
	if _, err := c.ctx.RefreshService(serviceName); err != nil {
		return nil, err
	}
	return c.ctx.ListenWithOptions(serviceName, &ziti.ListenOptions{
		DoNotSaveDialerIdentity:      false,
		ConnectTimeout:               5 * time.Minute,
		WaitForNEstablishedListeners: 1,
	})
}

func (c *openZitiContext) Dial(serviceName string) (net.Conn, error) {
	if _, err := c.ctx.RefreshService(serviceName); err != nil {
		return nil, err
	}
	return c.ctx.DialWithOptions(serviceName, &ziti.DialOptions{ConnectTimeout: 30 * time.Second})
}

func (c *openZitiContext) DialContext(ctx context.Context, serviceName string) (net.Conn, error) {
	return dialContext(ctx, func() (net.Conn, error) {
		return c.Dial(serviceName)
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
