package automation

import "time"

const (
	DefaultAgoraVersion     = "dev"
	DefaultRequestTimeout   = 30 * time.Second
	DefaultOperationTimeout = 30 * time.Second
)

type Config struct {
	APIEndpoint      string
	RequestTimeout   time.Duration
	OperationTimeout time.Duration
	Auth             AuthConfig
}

func (c *Config) normalized() Config {
	cfg := Config{}
	if c != nil {
		cfg = *c
	}
	if cfg.RequestTimeout == 0 {
		cfg.RequestTimeout = DefaultRequestTimeout
	}
	if cfg.OperationTimeout == 0 {
		cfg.OperationTimeout = DefaultOperationTimeout
	}
	return cfg
}

type AuthConfig struct {
	Mode string
	UPDB UPDBAuthConfig
}

type UPDBAuthConfig struct {
	Username string
	Password string
}
