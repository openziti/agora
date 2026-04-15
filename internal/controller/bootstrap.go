package controller

import (
	"context"
	"fmt"

	"github.com/michaelquigley/df/dl"
	"github.com/openziti/agora/internal/controller/config"
	"github.com/openziti/agora/internal/fabric/openziti/automation"
)

func Bootstrap(cfg *config.Config) error {
	dl.Info("connecting to the OpenZiti management API")
	client, err := openZitiClient(context.Background(), cfg)
	if err != nil {
		return err
	}

	dl.Info("connected to the OpenZiti management API")
	dl.Info("running OpenZiti bootstrap preflight checks")
	if err := automation.NewBootstrapper(client).Bootstrap(context.Background()); err != nil {
		return fmt.Errorf("bootstrap openziti resources: %w", err)
	}
	dl.Info("bootstrap preflight checks passed")

	return nil
}

func Unbootstrap(cfg *config.Config) error {
	dl.Info("connecting to the OpenZiti management API")
	client, err := openZitiClient(context.Background(), cfg)
	if err != nil {
		return err
	}

	dl.Info("connected to the OpenZiti management API")
	dl.Info("removing agora-owned OpenZiti resources")
	if err := automation.NewBootstrapper(client).Unbootstrap(context.Background()); err != nil {
		return fmt.Errorf("unbootstrap openziti resources: %w", err)
	}
	dl.Info("agora-owned OpenZiti resources removed")

	return nil
}

func openZitiClient(ctx context.Context, cfg *config.Config) (*automation.Client, error) {
	if cfg == nil {
		return nil, fmt.Errorf("controller config is required")
	}
	if cfg.OpenZiti == nil {
		return nil, fmt.Errorf("open_ziti config is required")
	}
	client, err := automation.NewClient(ctx, cfg.OpenZiti)
	if err != nil {
		return nil, fmt.Errorf("connect to the openziti management api: %w", err)
	}
	return client, nil
}
