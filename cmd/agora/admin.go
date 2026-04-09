package main

func init() {
	rootCmd.AddCommand(adminCmd)
	rootCmd.AddCommand(configCmd)
	adminCmd.AddCommand(adminCreateCmd)
	adminCmd.AddCommand(adminDeleteCmd)
	adminCmd.AddCommand(adminListCmd)
	adminCmd.AddCommand(storeCmd)
}
