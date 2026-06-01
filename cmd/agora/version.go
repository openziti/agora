package main

import (
	"fmt"

	"github.com/openziti/agora/build"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(newVersionCommand().cmd)
}

type versionCommand struct {
	cmd *cobra.Command
}

func newVersionCommand() *versionCommand {
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Show the agora version",
		Args:  cobra.NoArgs,
	}
	command := &versionCommand{cmd: cmd}
	cmd.Run = command.run
	return command
}

func (cmd *versionCommand) run(_ *cobra.Command, _ []string) {
	fmt.Println(build.String())
}
