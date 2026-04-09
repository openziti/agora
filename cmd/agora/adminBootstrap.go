package main

import (
	"github.com/michaelquigley/df/dd"
	"github.com/michaelquigley/df/dl"
	"github.com/openziti/agora/internal/controller"
	"github.com/openziti/agora/internal/controller/config"
	"github.com/spf13/cobra"
)

func init() {
	adminCmd.AddCommand(newAdminBootstrapCommand().cmd)
}

type adminBootstrapCommand struct {
	cmd *cobra.Command
}

func newAdminBootstrapCommand() *adminBootstrapCommand {
	cmd := &cobra.Command{
		Use:   "bootstrap <configPath>",
		Short: "Bootstrap the underlying OpenZiti network for agora",
		Args:  cobra.ExactArgs(1),
	}
	command := &adminBootstrapCommand{cmd: cmd}
	cmd.Run = command.run
	return command
}

func (cmd *adminBootstrapCommand) run(_ *cobra.Command, args []string) {
	cfg, err := config.Load(args[0])
	if err != nil {
		panic(err)
	}
	dl.Info(dd.MustInspect(cfg))
	if err := controller.Bootstrap(cfg); err != nil {
		panic(err)
	}
	dl.Info("bootstrap complete!")
}
