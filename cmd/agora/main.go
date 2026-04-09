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
	rootCmd.AddCommand(storeCmd)
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

var storeCmd = &cobra.Command{
	Use:   "store",
	Short: "Persistence and schema management commands",
}

var adminCmd = &cobra.Command{
	Use:   "admin",
	Short: "Administrative controller management commands",
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
