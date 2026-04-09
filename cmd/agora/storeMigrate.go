package main

import (
	"context"
	"fmt"
	"os"

	"github.com/michaelquigley/df/dl"
	ctrlcfg "github.com/openziti/agora/internal/controller/config"
	"github.com/openziti/agora/internal/persistence"
	"github.com/spf13/cobra"
)

func init() {
	storeCmd.AddCommand(newStoreMigrateCommand().cmd)
}

type storeMigrateCommand struct {
	cmd   *cobra.Command
	steps int
}

func newStoreMigrateCommand() *storeMigrateCommand {
	cmd := &cobra.Command{
		Use:   "migrate <configPath> <up|down|status>",
		Short: "Migrate the underlying datastore",
		Args:  cobra.ExactArgs(2),
	}
	command := &storeMigrateCommand{cmd: cmd}
	cmd.Flags().IntVar(&command.steps, "down", 0, "migrate down N steps (0 = migrate up/status)")
	cmd.Run = command.run
	return command
}

func (cmd *storeMigrateCommand) run(_ *cobra.Command, args []string) {
	cfg, err := ctrlcfg.Load(args[0])
	if err != nil {
		panic(err)
	}
	store := openStore(cfg.Store)
	defer func() { _ = store.Close() }()

	ctx := context.Background()

	switch args[1] {
	case "up":
		applied, err := persistence.MigrateUp(ctx, store)
		if err != nil {
			panic(err)
		}
		dl.Infof("migration complete; applied=%d", applied)
	case "down":
		steps := cmd.steps
		if steps <= 0 {
			panic("down migrations require --down > 0")
		}
		applied, err := persistence.MigrateDown(ctx, store, steps)
		if err != nil {
			panic(err)
		}
		dl.Infof("down migration complete; applied=%d", applied)
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
		panic("invalid migration action: " + args[1])
	}
}
