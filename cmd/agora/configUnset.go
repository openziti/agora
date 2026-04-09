package main

import (
	"fmt"

	"github.com/openziti/agora/environment"
	"github.com/spf13/cobra"
)

func init() {
	configCmd.AddCommand(newConfigUnsetCommand().cmd)
}

type configUnsetCommand struct {
	cmd *cobra.Command
}

func newConfigUnsetCommand() *configUnsetCommand {
	cmd := &cobra.Command{
		Use:   "unset <configName>",
		Short: "Unset a local environment configuration value",
		Args:  cobra.ExactArgs(1),
	}
	command := &configUnsetCommand{cmd: cmd}
	cmd.Run = command.run
	return command
}

func (cmd *configUnsetCommand) run(_ *cobra.Command, args []string) {
	root, err := environment.LoadRoot()
	if err != nil {
		panic(err)
	}
	cfg := root.Config()
	if cfg == nil {
		return
	}

	switch args[0] {
	case "api_endpoint":
		cfg.APIEndpoint = ""
		if err := root.SetConfig(cfg); err != nil {
			panic(err)
		}
		fmt.Println("agora configuration updated")
	default:
		panic("unknown config name: " + args[0])
	}
}
