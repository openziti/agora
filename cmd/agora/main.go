package main

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/michaelquigley/df/dl"
	"github.com/spf13/cobra"
	"log/slog"
)

func init() {
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose logging")
	rootCmd.PersistentFlags().BoolVarP(&panicInstead, "panic", "p", false, "Panic instead of showing pretty errors")
}

var rootCmd = &cobra.Command{
	Use:   strings.TrimSuffix(filepath.Base(os.Args[0]), filepath.Ext(os.Args[0])),
	Short: "agora",
	PersistentPreRun: func(_ *cobra.Command, _ []string) {
		level := slog.LevelInfo
		if verbose {
			level = slog.LevelDebug
		}
		dl.Init(dl.DefaultOptions().SetLevel(level).SetTrimPrefix("github.com/openziti/"))
	},
}

var (
	verbose      bool
	panicInstead bool
)

var adminCmd = &cobra.Command{
	Use:   "admin",
	Short: "Administrative controller management commands",
}

var adminCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Administrative resource creation commands",
}

var adminDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Administrative resource deletion commands",
}

var adminListCmd = &cobra.Command{
	Use:   "list",
	Short: "Administrative resource listing commands",
}

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Local environment configuration commands",
}

var storeCmd = &cobra.Command{
	Use:   "store",
	Short: "Persistence and schema management commands",
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		if panicInstead {
			panic(err)
		}
		_, _ = os.Stderr.WriteString("error: " + err.Error() + "\n")
		os.Exit(1)
	}
}
