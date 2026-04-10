package persistence

import "time"

type Config struct {
	DSN             string `dd:",+required,+secret"`
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
}
