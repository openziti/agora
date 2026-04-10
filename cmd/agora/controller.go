package main

import (
	"github.com/michaelquigley/df/dd"
	"github.com/michaelquigley/df/dl"
	"github.com/openziti/agora/internal/controller"
	"github.com/openziti/agora/internal/controller/config"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(newControllerCommand().cmd)
}

type controllerCommand struct {
	cmd *cobra.Command
}

func newControllerCommand() *controllerCommand {
	cmd := &cobra.Command{
		Use:     "controller <configPath>",
		Short:   "Start an agora controller",
		Aliases: []string{"ctrl"},
		Args:    cobra.ExactArgs(1),
	}
	command := &controllerCommand{cmd: cmd}
	cmd.Run = command.run
	return command
}

func (cmd *controllerCommand) run(_ *cobra.Command, args []string) {
	cfg, err := config.Load(args[0])
	if err != nil {
		panic(err)
	}
	dl.Info(dd.MustInspect(cfg))
	if err := controller.Run(cfg); err != nil {
		panic(err)
	}
}
