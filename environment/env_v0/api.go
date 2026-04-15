package env_v0

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/openziti/agora/environment/env_core"
	"github.com/openziti/agora/internal/api"
)

func (r *Root) Metadata() *env_core.Metadata {
	return r.meta
}

func (r *Root) HasConfig() (bool, error) {
	return r.cfg != nil, nil
}

func (r *Root) Config() *env_core.Config {
	return r.cfg
}

func (r *Root) SetConfig(cfg *env_core.Config) error {
	if err := assertMetadata(); err != nil {
		return err
	}
	if err := saveConfig(cfg); err != nil {
		return err
	}
	r.cfg = cfg
	return nil
}

func (r *Root) Client() (*api.Client, error) {
	apiEndpoint, _ := r.APIEndpoint()
	if apiEndpoint == "" {
		return nil, fmt.Errorf("api endpoint is not configured")
	}
	baseURL := strings.TrimRight(apiEndpoint, "/") + "/v1"
	return api.NewClient(baseURL, noAuthSecuritySource{})
}

func (r *Root) APIEndpoint() (string, string) {
	if env := os.Getenv("AGORA_API_ENDPOINT"); env != "" {
		return env, "AGORA_API_ENDPOINT"
	}
	if r.IsEnabled() && r.Environment().APIEndpoint != "" {
		return r.Environment().APIEndpoint, "env"
	}
	if r.Config() != nil && r.Config().APIEndpoint != "" {
		return r.Config().APIEndpoint, "config"
	}
	return "", "unset"
}

func (r *Root) Environment() *env_core.Environment {
	return r.env
}

func (r *Root) SetEnvironment(env *env_core.Environment) error {
	if err := assertMetadata(); err != nil {
		return err
	}
	if err := saveEnvironment(env); err != nil {
		return err
	}
	r.env = env
	return nil
}

func (r *Root) DeleteEnvironment() error {
	path, err := environmentFile()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	r.env = nil
	return nil
}

func (r *Root) IsEnabled() bool {
	return r.env != nil
}

func (r *Root) Network() *env_core.Network {
	return r.net
}

func (r *Root) SetNetwork(network *env_core.Network) error {
	if err := assertMetadata(); err != nil {
		return err
	}
	if err := saveNetwork(network); err != nil {
		return err
	}
	r.net = network
	return nil
}

func (r *Root) DeleteNetwork() error {
	path, err := networkFile()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	r.net = nil
	return nil
}

func (r *Root) NetworkSocketPath() (string, error) {
	return networkSocketFile()
}

func (r *Root) ZitiIdentityNamed(name string) (string, error) {
	return identityFile(name)
}

func (r *Root) SaveZitiIdentityNamed(name, data string) error {
	if err := assertMetadata(); err != nil {
		return err
	}
	path, err := identityFile(name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(data), 0o600)
}

func (r *Root) DeleteZitiIdentityNamed(name string) error {
	path, err := identityFile(name)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (r *Root) Obliterate() error {
	root, err := rootDir()
	if err != nil {
		return err
	}
	return os.RemoveAll(root)
}

type noAuthSecuritySource struct{}

func (noAuthSecuritySource) AccountTokenAuth(context.Context, api.OperationName) (api.AccountTokenAuth, error) {
	return api.AccountTokenAuth{}, nil
}

func (noAuthSecuritySource) AdminTokenAuth(context.Context, api.OperationName) (api.AdminTokenAuth, error) {
	return api.AdminTokenAuth{}, nil
}

func IsValidAPIEndpoint(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return err
	}
	if u.Scheme == "" || u.Host == "" {
		return fmt.Errorf("api endpoint must include scheme and host")
	}
	return nil
}
