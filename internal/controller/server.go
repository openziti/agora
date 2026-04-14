package controller

import (
	"context"
	"fmt"
	"net/http"

	"github.com/michaelquigley/df/dl"
	"github.com/openziti/agora/internal/controller/config"
	"github.com/openziti/agora/internal/persistence"
)

type Controller struct {
	cfg     *config.Config
	store   *persistence.Store
	service *Service
}

func New(cfg *config.Config) (*Controller, error) {
	dl.Infof("opening persistence store for controller bind_address='%s'", cfg.BindAddress)
	store, err := persistence.Open(context.Background(), cfg.Store)
	if err != nil {
		return nil, err
	}
	dl.Infof("opened persistence store for controller bind_address='%s'", cfg.BindAddress)
	service := NewService(cfg, store)
	return &Controller{
		cfg:     cfg,
		store:   store,
		service: service,
	}, nil
}

func Run(cfg *config.Config) error {
	dl.Infof("starting agora controller bind_address='%s'", cfg.BindAddress)
	controller, err := New(cfg)
	if err != nil {
		return fmt.Errorf("create controller: %w", err)
	}
	defer func() {
		if err := controller.store.Close(); err != nil {
			dl.Errorf("error closing controller store: %v", err)
		}
	}()

	dl.Info("checking schema compatibility")
	if err := persistence.CheckSchemaCompatibility(context.Background(), controller.store); err != nil {
		return fmt.Errorf("check schema compatibility: %w", err)
	}
	dl.Info("schema compatibility check passed")

	dl.Info("building controller http handler")
	handler, err := NewHandler(controller.service)
	if err != nil {
		return fmt.Errorf("build handler: %w", err)
	}
	go controller.service.RunTunnelAttachmentReaper(context.Background())
	go controller.service.RunTunnelServeReaper(context.Background())
	dl.Infof("controller http handler ready; listening on '%s'", controller.cfg.BindAddress)

	return http.ListenAndServe(controller.cfg.BindAddress, handler)
}
