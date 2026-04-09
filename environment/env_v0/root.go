package env_v0

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/openziti/agora/environment/env_core"
)

const V = "v0"

type Root struct {
	meta *env_core.Metadata
	cfg  *env_core.Config
	env  *env_core.Environment
}

type metadata struct {
	V string `json:"v"`
}

type config struct {
	APIEndpoint string `json:"api_endpoint"`
}

type environment struct {
	AccountToken string `json:"account_token"`
	ZitiIdentity string `json:"ziti_identity"`
	APIEndpoint  string `json:"api_endpoint"`
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
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	m := &metadata{}
	if err := json.Unmarshal(data, m); err != nil {
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
	data, err := json.Marshal(&metadata{V: V})
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func loadConfig() (*env_core.Config, error) {
	path, err := configFile()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	cfg := &config{}
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	return &env_core.Config{APIEndpoint: cfg.APIEndpoint}, nil
}

func saveConfig(cfg *env_core.Config) error {
	path, err := configFile()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(&config{APIEndpoint: cfg.APIEndpoint}, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func loadEnvironment() (*env_core.Environment, error) {
	path, err := environmentFile()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	env := &environment{}
	if err := json.Unmarshal(data, env); err != nil {
		return nil, err
	}
	return &env_core.Environment{
		AccountToken: env.AccountToken,
		ZitiIdentity: env.ZitiIdentity,
		APIEndpoint:  env.APIEndpoint,
	}, nil
}

func saveEnvironment(env *env_core.Environment) error {
	path, err := environmentFile()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(&environment{
		AccountToken: env.AccountToken,
		ZitiIdentity: env.ZitiIdentity,
		APIEndpoint:  env.APIEndpoint,
	}, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}
