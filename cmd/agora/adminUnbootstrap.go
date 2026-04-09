package main

import (
	"github.com/michaelquigley/df/dd"
	"github.com/michaelquigley/df/dl"
	"github.com/openziti/agora/internal/controller"
	"github.com/openziti/agora/internal/controller/config"
	"github.com/spf13/cobra"
)

func init() {
	adminCmd.AddCommand(newAdminUnbootstrapCommand().cmd)
}

type adminUnbootstrapCommand struct {
	cmd *cobra.Command
}

func newAdminUnbootstrapCommand() *adminUnbootstrapCommand {
	cmd := &cobra.Command{
		Use:   "unbootstrap <configPath>",
		Short: "Remove agora-owned OpenZiti resources from the underlying network",
		Args:  cobra.ExactArgs(1),
	}
	command := &adminUnbootstrapCommand{cmd: cmd}
	cmd.Run = command.run
	return command
}

func (cmd *adminUnbootstrapCommand) run(_ *cobra.Command, args []string) {
	cfg, err := config.Load(args[0])
	if err != nil {
		panic(err)
	}
	dl.Info(dd.MustInspect(cfg))
	if err := controller.Unbootstrap(cfg); err != nil {
		panic(err)
	}
	dl.Info("unbootstrap complete!")
}
