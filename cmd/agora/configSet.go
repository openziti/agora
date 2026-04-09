package main

import (
	"fmt"

	"github.com/openziti/agora/environment"
	"github.com/openziti/agora/environment/env_core"
	"github.com/openziti/agora/environment/env_v0"
	"github.com/spf13/cobra"
)

func init() {
	configCmd.AddCommand(newConfigSetCommand().cmd)
}

type configSetCommand struct {
	cmd *cobra.Command
}

func newConfigSetCommand() *configSetCommand {
	cmd := &cobra.Command{
		Use:   "set <configName> <value>",
		Short: "Set a local environment configuration value",
		Args:  cobra.ExactArgs(2),
	}
	command := &configSetCommand{cmd: cmd}
	cmd.Run = command.run
	return command
}

func (cmd *configSetCommand) run(_ *cobra.Command, args []string) {
	root, err := environment.LoadRoot()
	if err != nil {
		panic(err)
	}

	switch args[0] {
	case "api_endpoint":
		if err := env_v0.IsValidAPIEndpoint(args[1]); err != nil {
			panic(err)
		}
		cfg := root.Config()
		if cfg == nil {
			cfg = &env_core.Config{}
		}
		cfg.APIEndpoint = args[1]
		if err := root.SetConfig(cfg); err != nil {
			panic(err)
		}
		fmt.Println("agora configuration updated")
	default:
		panic("unknown config name: " + args[0])
	}
}
