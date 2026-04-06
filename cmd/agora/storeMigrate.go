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
	storeCmd.AddCommand(newStoreMigrateCommand().cmd)
}

type storeMigrateCommand struct {
	cmd             *cobra.Command
	dsn             string
	maxOpenConns    int
	maxIdleConns    int
	connMaxLifetime time.Duration
	steps           int
}

func newStoreMigrateCommand() *storeMigrateCommand {
	cmd := &cobra.Command{
		Use:   "migrate <up|down|status>",
		Short: "Migrate the underlying datastore",
		Args:  cobra.ExactArgs(1),
	}
	command := &storeMigrateCommand{cmd: cmd}
	cmd.Flags().StringVar(&command.dsn, "dsn", os.Getenv("AGORA_DATABASE_URL"), "PostgreSQL connection string")
	cmd.Flags().IntVar(&command.maxOpenConns, "max-open-conns", 4, "Maximum number of open database connections")
	cmd.Flags().IntVar(&command.maxIdleConns, "max-idle-conns", 4, "Maximum number of idle database connections")
	cmd.Flags().DurationVar(&command.connMaxLifetime, "conn-max-lifetime", time.Hour, "Maximum database connection lifetime")
	cmd.Flags().IntVar(&command.steps, "down", 0, "migrate down N steps (0 = migrate up/status)")
	cmd.Run = command.run
	return command
}

func (cmd *storeMigrateCommand) run(_ *cobra.Command, args []string) {
	store := openStore(cmd.storeConfig())
	defer func() { _ = store.Close() }()

	ctx := context.Background()

	switch args[0] {
	case "up":
		applied, err := persistence.MigrateUp(ctx, store)
		if err != nil {
			panic(err)
		}
		logger.Info("migration complete", "applied", applied)
	case "down":
		steps := cmd.steps
		if steps <= 0 {
			panic("down migrations require --down > 0")
		}
		applied, err := persistence.MigrateDown(ctx, store, steps)
		if err != nil {
			panic(err)
		}
		logger.Info("down migration complete", "applied", applied)
	case "status":
		statuses, err := persistence.MigrationStatusList(ctx, store)
		if err != nil {
			if err == persistence.ErrNoMigrationsTable {
				_, _ = fmt.Fprintln(os.Stdout, "migrations table missing")
				return
			}
			panic(err)
		}
		for _, status := range statuses {
			_, _ = fmt.Fprintf(os.Stdout, "%s\t%s\n", status.ID, appliedLabel(status.Applied))
		}
	default:
		panic("invalid migration action: " + args[0])
	}
}

func (cmd *storeMigrateCommand) storeConfig() persistence.Config {
	return persistence.Config{
		DSN:             cmd.dsn,
		MaxOpenConns:    cmd.maxOpenConns,
		MaxIdleConns:    cmd.maxIdleConns,
		ConnMaxLifetime: cmd.connMaxLifetime,
	}
}
