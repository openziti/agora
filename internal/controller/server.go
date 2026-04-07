package controller

import (
	"context"
	"fmt"
	"net/http"

	"github.com/openziti/agora/internal/controller/config"
	"github.com/openziti/agora/internal/persistence"
)

type Controller struct {
	cfg     *config.Config
	store   *persistence.Store
	service *Service
}

func New(cfg *config.Config) (*Controller, error) {
	store, err := persistence.Open(context.Background(), cfg.Store)
	if err != nil {
		return nil, err
	}
	service := NewService(cfg, store)
	return &Controller{
		cfg:     cfg,
		store:   store,
		service: service,
	}, nil
}

func Run(cfg *config.Config) error {
	controller, err := New(cfg)
	if err != nil {
		return fmt.Errorf("create controller: %w", err)
	}
	defer func() { _ = controller.store.Close() }()

	if err := persistence.CheckSchemaCompatibility(context.Background(), controller.store); err != nil {
		return fmt.Errorf("check schema compatibility: %w", err)
	}

	handler, err := NewHandler(controller.service)
	if err != nil {
		return fmt.Errorf("build handler: %w", err)
	}

	return http.ListenAndServe(controller.cfg.BindAddress, handler)
}
