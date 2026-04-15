package automation

import (
	"context"
	"fmt"

	"github.com/michaelquigley/df/dl"
)

type Bootstrapper struct {
	edgeRouters EdgeRouterOperations
	identities  IdentityOperations
	cleanup     func(context.Context, string) error
}

func NewBootstrapper(client *Client) *Bootstrapper {
	return &Bootstrapper{
		edgeRouters: client.EdgeRouters,
		identities:  client.Identities,
		cleanup:     client.CleanupByTag,
	}
}

func (b *Bootstrapper) Bootstrap(ctx context.Context) error {
	dl.Debug("verifying OpenZiti management API identity access")
	if _, err := b.identities.Find(ctx, &FilterOptions{Limit: 1}); err != nil {
		return fmt.Errorf("verify openziti management connectivity: %w", err)
	}
	dl.Debug("verifying OpenZiti edge router availability")
	edgeRouters, err := b.edgeRouters.Find(ctx, &FilterOptions{Limit: 1})
	if err != nil {
		return fmt.Errorf("list edge routers: %w", err)
	}
	if len(edgeRouters) == 0 {
		return fmt.Errorf("no edge routers are available")
	}
	dl.Debugf("found %d edge router(s) during bootstrap preflight", len(edgeRouters))
	return nil
}

func (b *Bootstrapper) Unbootstrap(ctx context.Context) error {
	dl.Debug("cleaning up OpenZiti resources tagged with agora")
	if err := b.cleanup(ctx, "agora"); err != nil {
		return fmt.Errorf("cleanup agora-tagged resources: %w", err)
	}
	return nil
}
