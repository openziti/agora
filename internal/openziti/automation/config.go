package automation

import "time"

const (
	DefaultAgoraVersion     = "dev"
	DefaultRequestTimeout   = 30 * time.Second
	DefaultOperationTimeout = 30 * time.Second
)

type Config struct {
	APIEndpoint      string `dd:",+required"`
	RequestTimeout   time.Duration
	OperationTimeout time.Duration
	Auth             AuthConfig
}

func (c *Config) ApplyDefaults() {
	if c.RequestTimeout == 0 {
		c.RequestTimeout = DefaultRequestTimeout
	}
	if c.OperationTimeout == 0 {
		c.OperationTimeout = DefaultOperationTimeout
	}
	if c.Auth.Mode == "" {
		c.Auth.Mode = "updb"
	}
}

type AuthConfig struct {
	Mode string
	UPDB UPDBAuthConfig
}

type UPDBAuthConfig struct {
	Username string
	Password string
}
