package persistence

import "time"

type Config struct {
	DSN             string `dd:",+required"`
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
}
