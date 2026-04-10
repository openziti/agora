package env_core

import "github.com/openziti/agora/internal/api"

type Root interface {
	Metadata() *Metadata
	Obliterate() error

	HasConfig() (bool, error)
	Config() *Config
	SetConfig(cfg *Config) error

	Client() (*api.Client, error)
	APIEndpoint() (string, string)

	IsEnabled() bool
	Environment() *Environment
	SetEnvironment(env *Environment) error
	DeleteEnvironment() error

	ZitiIdentityNamed(name string) (string, error)
	SaveZitiIdentityNamed(name, data string) error
	DeleteZitiIdentityNamed(name string) error
}

type Environment struct {
	EnvironmentID string
	AccountToken  string
	ZitiIdentity  string
	APIEndpoint   string
}

type Config struct {
	APIEndpoint string
}

type Metadata struct {
	V        string
	RootPath string
}
