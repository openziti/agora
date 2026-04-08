package automation

import (
	"context"
	"fmt"
)

type Bootstrapper struct {
	identities IdentityOperations
}

func NewBootstrapper(client *Client) *Bootstrapper {
	return &Bootstrapper{identities: client.Identities}
}

func (b *Bootstrapper) Bootstrap(ctx context.Context) error {
	if _, err := b.identities.Find(ctx, &FilterOptions{Limit: 1}); err != nil {
		return fmt.Errorf("verify openziti management connectivity: %w", err)
	}
	return nil
}
