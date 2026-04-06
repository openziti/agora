package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/openziti/agora/internal/persistence"
	"github.com/spf13/cobra"
)

func init() {
	storeCmd.AddCommand(newStoreCheckSchemaCommand().cmd)
}

type storeCheckSchemaCommand struct {
	cmd             *cobra.Command
	dsn             string
	maxOpenConns    int
	maxIdleConns    int
	connMaxLifetime time.Duration
}

func newStoreCheckSchemaCommand() *storeCheckSchemaCommand {
	cmd := &cobra.Command{
		Use:   "check-schema",
		Short: "Verify the datastore schema is compatible with this binary",
		Args:  cobra.ExactArgs(0),
	}
	command := &storeCheckSchemaCommand{cmd: cmd}
	cmd.Flags().StringVar(&command.dsn, "dsn", os.Getenv("AGORA_DATABASE_URL"), "PostgreSQL connection string")
	cmd.Flags().IntVar(&command.maxOpenConns, "max-open-conns", 4, "Maximum number of open database connections")
	cmd.Flags().IntVar(&command.maxIdleConns, "max-idle-conns", 4, "Maximum number of idle database connections")
	cmd.Flags().DurationVar(&command.connMaxLifetime, "conn-max-lifetime", time.Hour, "Maximum database connection lifetime")
	cmd.Run = command.run
	return command
}

func (cmd *storeCheckSchemaCommand) run(_ *cobra.Command, _ []string) {
	store := openStore(persistence.Config{
		DSN:             cmd.dsn,
		MaxOpenConns:    cmd.maxOpenConns,
		MaxIdleConns:    cmd.maxIdleConns,
		ConnMaxLifetime: cmd.connMaxLifetime,
	})
	defer func() { _ = store.Close() }()

	if err := persistence.CheckSchemaCompatibility(context.Background(), store); err != nil {
		panic(err)
	}
	_, _ = fmt.Fprintln(os.Stdout, "schema compatible")
}
