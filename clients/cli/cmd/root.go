package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	version = "dev"
	commit  = "unknown"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "progressdb",
	Short: "ProgressDB CLI tool for database management",
	Long: `ProgressDB CLI provides tools for managing ProgressDB databases,
including migration between different versions.`,
	Version: fmt.Sprintf("%s (commit: %s)", version, commit),
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func init() {
	// Global flags
	rootCmd.PersistentFlags().BoolP("verbose", "v", false, "enable verbose output")
	rootCmd.PersistentFlags().StringP("config", "c", "", "config file path (default is $HOME/.progressdb.yaml)")
}
