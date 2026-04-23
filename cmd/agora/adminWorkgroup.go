package main

import (
	"github.com/spf13/cobra"
)

var adminWorkgroupCmd = &cobra.Command{
	Use:   "workgroup",
	Short: "Admin-side workgroup commands",
}

func init() {
	adminCmd.AddCommand(adminWorkgroupCmd)
}
