package env_v0

import (
	"os"
	"path/filepath"

	"github.com/michaelquigley/df/dd"
	"github.com/openziti/agora/environment/env_core"
	"github.com/openziti/agora/internal/api"
)

const V = "v0"

type Root struct {
	meta *env_core.Metadata
	cfg  *env_core.Config
	env  *env_core.Environment
	net  *env_core.Network
}

type metadata struct {
	V string `dd:"v"`
}

type config struct {
	APIEndpoint string
}

type environment struct {
	EnvironmentID string
	AccountToken  string
	ZitiIdentity  string
	APIEndpoint   string
}

type network struct {
	Serves   []managedServe   `dd:",+omitempty"`
	Connects []managedConnect `dd:",+omitempty"`
}

type managedServe struct {
	TunnelID      string `dd:",+omitempty"`
	Name          string
	Mode          string
	BackendTarget string
	GrantEmails   []string `dd:",+omitempty"`
}

type managedConnect struct {
	TunnelID      string `dd:",+omitempty"`
	Name          string
	ListenAddress string
}

func Default() (*Root, error) {
	root, err := rootDir()
	if err != nil {
		return nil, err
	}
	return &Root{
		meta: &env_core.Metadata{
			V:        V,
			RootPath: root,
		},
	}, nil
}

func Assert() (bool, error) {
	exists, err := rootExists()
	if err != nil {
		return true, err
	}
	if !exists {
		return false, nil
	}
	meta, err := loadMetadata()
	if err != nil {
		return true, err
	}
	return meta.V == V, nil
}

func Load() (*Root, error) {
	exists, err := rootExists()
	if err != nil {
		return nil, err
	}
	if !exists {
		return Default()
	}

	meta, err := loadMetadata()
	if err != nil {
		return nil, err
	}
	r := &Root{meta: meta}

	if cfg, err := loadConfig(); err == nil {
		r.cfg = cfg
	}
	if env, err := loadEnvironment(); err == nil {
		r.env = env
	}
	if net, err := loadNetwork(); err == nil {
		r.net = net
	}

	return r, nil
}

func rootExists() (bool, error) {
	path, err := metadataFile()
	if err != nil {
		return false, err
	}
	_, err = os.Stat(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	return err == nil, err
}

func assertMetadata() error {
	exists, err := rootExists()
	if err != nil {
		return err
	}
	if !exists {
		return writeMetadata()
	}
	return nil
}

func loadMetadata() (*env_core.Metadata, error) {
	path, err := metadataFile()
	if err != nil {
		return nil, err
	}
	m := &metadata{}
	if err := dd.BindJSONFile(m, path); err != nil {
		return nil, err
	}
	root, err := rootDir()
	if err != nil {
		return nil, err
	}
	return &env_core.Metadata{V: m.V, RootPath: root}, nil
}

func writeMetadata() error {
	path, err := metadataFile()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return dd.UnbindJSONFile(&metadata{V: V}, path, rootJSONFileOptions())
}

func loadConfig() (*env_core.Config, error) {
	path, err := configFile()
	if err != nil {
		return nil, err
	}
	cfg := &config{}
	if err := dd.BindJSONFile(cfg, path); err != nil {
		return nil, err
	}
	return &env_core.Config{APIEndpoint: cfg.APIEndpoint}, nil
}

func saveConfig(cfg *env_core.Config) error {
	path, err := configFile()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return dd.UnbindJSONFile(&config{APIEndpoint: cfg.APIEndpoint}, path, rootJSONFileOptions())
}

func loadEnvironment() (*env_core.Environment, error) {
	path, err := environmentFile()
	if err != nil {
		return nil, err
	}
	env := &environment{}
	if err := dd.BindJSONFile(env, path); err != nil {
		return nil, err
	}
	return &env_core.Environment{
		EnvironmentID: env.EnvironmentID,
		AccountToken:  env.AccountToken,
		ZitiIdentity:  env.ZitiIdentity,
		APIEndpoint:   env.APIEndpoint,
	}, nil
}

func saveEnvironment(env *env_core.Environment) error {
	path, err := environmentFile()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return dd.UnbindJSONFile(&environment{
		EnvironmentID: env.EnvironmentID,
		AccountToken:  env.AccountToken,
		ZitiIdentity:  env.ZitiIdentity,
		APIEndpoint:   env.APIEndpoint,
	}, path, rootJSONFileOptions())
}

func loadNetwork() (*env_core.Network, error) {
	path, err := networkFile()
	if err != nil {
		return nil, err
	}
	n := &network{}
	if err := dd.BindJSONFile(n, path); err != nil {
		return nil, err
	}
	result := &env_core.Network{
		Serves:   make([]env_core.ManagedServe, 0, len(n.Serves)),
		Connects: make([]env_core.ManagedConnect, 0, len(n.Connects)),
	}
	for _, serve := range n.Serves {
		result.Serves = append(result.Serves, env_core.ManagedServe{
			TunnelID:      serve.TunnelID,
			Name:          serve.Name,
			Mode:          api.TunnelMode(serve.Mode),
			BackendTarget: serve.BackendTarget,
			GrantEmails:   append([]string(nil), serve.GrantEmails...),
		})
	}
	for _, connect := range n.Connects {
		result.Connects = append(result.Connects, env_core.ManagedConnect{
			TunnelID:      connect.TunnelID,
			Name:          connect.Name,
			ListenAddress: connect.ListenAddress,
		})
	}
	return result, nil
}

func saveNetwork(networkState *env_core.Network) error {
	path, err := networkFile()
	if err != nil {
		return err
	}
	n := &network{
		Serves:   make([]managedServe, 0, len(networkState.Serves)),
		Connects: make([]managedConnect, 0, len(networkState.Connects)),
	}
	for _, serve := range networkState.Serves {
		n.Serves = append(n.Serves, managedServe{
			TunnelID:      serve.TunnelID,
			Name:          serve.Name,
			Mode:          string(serve.Mode),
			BackendTarget: serve.BackendTarget,
			GrantEmails:   append([]string(nil), serve.GrantEmails...),
		})
	}
	for _, connect := range networkState.Connects {
		n.Connects = append(n.Connects, managedConnect{
			TunnelID:      connect.TunnelID,
			Name:          connect.Name,
			ListenAddress: connect.ListenAddress,
		})
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return dd.UnbindJSONFile(n, path, rootJSONFileOptions())
}

func rootJSONFileOptions() *dd.Options {
	mode := os.FileMode(0o600)
	return &dd.Options{
		File: &dd.FileOptions{
			Mode: &mode,
		},
	}
}
