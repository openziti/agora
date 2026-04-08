package config

import (
	"fmt"
	"time"

	"github.com/michaelquigley/df/dd"
	"github.com/openziti/agora/internal/openziti/automation"
	"github.com/openziti/agora/internal/persistence"
)

type Config struct {
	BindAddress string
	AdminTokens []string
	OpenZiti    *automation.Config
	Store       persistence.Config `dd:",+required"`
}

func DefaultConfig() *Config {
	return &Config{
		BindAddress: ":8080",
		AdminTokens: nil,
		Store: persistence.Config{
			MaxOpenConns:    4,
			MaxIdleConns:    4,
			ConnMaxLifetime: time.Hour,
		},
	}
}

func Load(path string) (*Config, error) {
	cfg := DefaultConfig()
	if err := dd.MergeYAMLFile(cfg, path, &dd.Options{}); err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	return cfg, nil
}
