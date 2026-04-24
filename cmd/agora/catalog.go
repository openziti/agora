package main

import (
	"github.com/spf13/cobra"
)

var catalogCmd = &cobra.Command{
	Use:   "catalog",
	Short: "Discover advertisements published by other agents",
}

func init() {
	rootCmd.AddCommand(catalogCmd)
}
