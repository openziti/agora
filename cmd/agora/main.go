package main

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
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
		if verbose {
			logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
		}
	},
}

var (
	verbose      bool
	panicInstead bool
	logger       = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
)

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
