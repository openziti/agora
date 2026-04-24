package main

import "github.com/spf13/cobra"

var adminSessionCmd = &cobra.Command{
	Use:   "session",
	Short: "Administrator operations on sessions",
}

func init() {
	adminCmd.AddCommand(adminSessionCmd)
}
