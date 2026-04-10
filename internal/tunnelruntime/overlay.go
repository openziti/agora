package tunnelruntime

import (
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
	return c.ctx.ListenWithOptions(serviceName, &ziti.ListenOptions{
		ConnectTimeout:               5 * time.Minute,
		WaitForNEstablishedListeners: 1,
	})
}

func (c *openZitiContext) Dial(serviceName string) (net.Conn, error) {
	return c.ctx.DialWithOptions(serviceName, &ziti.DialOptions{ConnectTimeout: 30 * time.Second})
}
