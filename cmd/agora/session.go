package main

import (
	"github.com/spf13/cobra"
)

var sessionCmd = &cobra.Command{
	Use:   "session",
	Short: "Manage collaboration sessions",
}

func init() {
	rootCmd.AddCommand(sessionCmd)
}
