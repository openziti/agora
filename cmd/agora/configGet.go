package main

import (
	"fmt"

	"github.com/openziti/agora/environment"
	"github.com/spf13/cobra"
)

func init() {
	configCmd.AddCommand(newConfigGetCommand().cmd)
}

type configGetCommand struct {
	cmd *cobra.Command
}

func newConfigGetCommand() *configGetCommand {
	cmd := &cobra.Command{
		Use:   "get <configName>",
		Short: "Get a local environment configuration value",
		Args:  cobra.ExactArgs(1),
	}
	command := &configGetCommand{cmd: cmd}
	cmd.Run = command.run
	return command
}

func (cmd *configGetCommand) run(_ *cobra.Command, args []string) {
	root, err := environment.LoadRoot()
	if err != nil {
		panic(err)
	}

	switch args[0] {
	case "api_endpoint":
		value, _ := root.APIEndpoint()
		if value == "" {
			fmt.Println("api_endpoint = <unset>")
			return
		}
		fmt.Printf("api_endpoint = %s\n", value)
	default:
		panic("unknown config name: " + args[0])
	}
}
