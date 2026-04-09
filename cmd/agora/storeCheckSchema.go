package main

import (
	"context"
	"fmt"
	"os"

	ctrlcfg "github.com/openziti/agora/internal/controller/config"
	"github.com/openziti/agora/internal/persistence"
	"github.com/spf13/cobra"
)

func init() {
	storeCmd.AddCommand(newStoreCheckSchemaCommand().cmd)
}

type storeCheckSchemaCommand struct {
	cmd *cobra.Command
}

func newStoreCheckSchemaCommand() *storeCheckSchemaCommand {
	cmd := &cobra.Command{
		Use:   "check-schema <configPath>",
		Short: "Verify the datastore schema is compatible with this binary",
		Args:  cobra.ExactArgs(1),
	}
	command := &storeCheckSchemaCommand{cmd: cmd}
	cmd.Run = command.run
	return command
}

func (cmd *storeCheckSchemaCommand) run(_ *cobra.Command, args []string) {
	cfg, err := ctrlcfg.Load(args[0])
	if err != nil {
		panic(err)
	}
	store := openStore(cfg.Store)
	defer func() { _ = store.Close() }()

	if err := persistence.CheckSchemaCompatibility(context.Background(), store); err != nil {
		panic(err)
	}
	_, _ = fmt.Fprintln(os.Stdout, "schema compatible")
}
