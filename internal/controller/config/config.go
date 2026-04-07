package config

import (
	"fmt"
	"os"
	"time"

	"github.com/openziti/agora/internal/persistence"
	"gopkg.in/yaml.v3"
)

type Config struct {
	BindAddress string             `yaml:"bindAddress"`
	AdminTokens []string           `yaml:"adminTokens"`
	Store       persistence.Config `yaml:"store"`
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
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	return cfg, nil
}
