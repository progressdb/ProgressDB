package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"progressdb/clients/cli/config"
	"progressdb/clients/cli/internal/migrate"
	"progressdb/clients/cli/internal/prompt"

	"github.com/spf13/cobra"
)

// migrateCmd represents the migrate command
var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Migrate ProgressDB database between versions",
	Long: `Migrate ProgressDB database from one version to another.
Currently supports migration from 0.1.2 to 0.5.0.

Example usage:
  progressdb migrate --from /old/db --to /new/db --config config.yaml
  progressdb migrate --from /old/db --to /new/db  # will prompt for missing info`,
	RunE: runMigrate,
}

var (
	fromPath     string
	toPath       string
	configFile   string
	interactive  bool
	outputFormat string
)

func init() {
	rootCmd.AddCommand(migrateCmd)

	migrateCmd.Flags().StringVar(&fromPath, "from", "", "path to source database (required)")
	migrateCmd.Flags().StringVar(&toPath, "to", "", "path to target database (required)")
	migrateCmd.Flags().StringVar(&configFile, "config", "", "configuration file path")
	migrateCmd.Flags().StringVar(&outputFormat, "format", "json", "output format: json or pebble (default: json)")
	migrateCmd.Flags().BoolVar(&interactive, "interactive", true, "enable interactive prompts for missing values")

	migrateCmd.MarkFlagRequired("from")
	migrateCmd.MarkFlagRequired("to")
}

func runMigrate(cmd *cobra.Command, args []string) error {
	verbose, _ := cmd.Flags().GetBool("verbose")

	// Load configuration
	cfg, err := loadConfiguration()
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	// Validate configuration
	if err := validateConfiguration(cfg); err != nil {
		return fmt.Errorf("configuration validation failed: %w", err)
	}

	// Show migration summary
	if err := showMigrationSummary(cfg, verbose); err != nil {
		return err
	}

	// Confirm migration
	if !confirmMigration() {
		fmt.Println("Migration cancelled.")
		return nil
	}

	// Run migration
	if err := migrate.Run(cfg, verbose); err != nil {
		return fmt.Errorf("migration failed: %w", err)
	}

	fmt.Println("Migration completed successfully!")
	return nil
}

func loadConfiguration() (*config.Config, error) {
	var cfg *config.Config
	var err error

	// Try to load from config file first
	if configFile != "" {
		cfg, err = config.LoadFromFile(configFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load config file: %w", err)
		}
	} else {
		cfg = &config.Config{}
	}

	// Override with command line flags
	if fromPath != "" {
		cfg.FromDatabase = fromPath
	}
	if toPath != "" {
		cfg.ToDatabase = toPath
	}
	if outputFormat != "" {
		cfg.OutputFormat = outputFormat
	}

	// Use interactive prompts for missing values
	if interactive {
		if err := prompt.FillMissing(cfg); err != nil {
			return nil, fmt.Errorf("interactive configuration failed: %w", err)
		}
	}

	return cfg, nil
}

func validateConfiguration(cfg *config.Config) error {
	if cfg.FromDatabase == "" {
		return fmt.Errorf("source database path is required")
	}
	if cfg.ToDatabase == "" {
		return fmt.Errorf("target database path is required")
	}
	if cfg.OldEncryptionKey == "" {
		return fmt.Errorf("old encryption key is required for migration")
	}

	// Validate output format
	if cfg.OutputFormat == "" {
		cfg.OutputFormat = "json" // default to JSON
	}
	if cfg.OutputFormat != "json" && cfg.OutputFormat != "pebble" {
		return fmt.Errorf("invalid output format '%s', must be 'json' or 'pebble'", cfg.OutputFormat)
	}

	// Validate paths
	if !filepath.IsAbs(cfg.FromDatabase) {
		abs, err := filepath.Abs(cfg.FromDatabase)
		if err != nil {
			return fmt.Errorf("invalid source database path: %w", err)
		}
		cfg.FromDatabase = abs
	}
	if !filepath.IsAbs(cfg.ToDatabase) {
		abs, err := filepath.Abs(cfg.ToDatabase)
		if err != nil {
			return fmt.Errorf("invalid target database path: %w", err)
		}
		cfg.ToDatabase = abs
	}

	// Check if source database exists
	if _, err := os.Stat(cfg.FromDatabase); os.IsNotExist(err) {
		return fmt.Errorf("source database does not exist: %s", cfg.FromDatabase)
	}

	// Validate encryption key format
	if err := config.ValidateEncryptionKey(cfg.OldEncryptionKey); err != nil {
		return fmt.Errorf("invalid encryption key: %w", err)
	}

	return nil
}

func showMigrationSummary(cfg *config.Config, verbose bool) error {
	fmt.Println("Migration Summary:")
	fmt.Printf("  Source:      %s\n", cfg.FromDatabase)
	fmt.Printf("  Target:      %s\n", cfg.ToDatabase)
	fmt.Printf("  Format:      %s\n", cfg.OutputFormat)
	fmt.Printf("  Encryption:  %s\n", maskKey(cfg.OldEncryptionKey))

	if verbose {
		fmt.Printf("  Config File: %s\n", configFile)
	}

	fmt.Println()
	return nil
}

func confirmMigration() bool {
	fmt.Print("Do you want to proceed with the migration? [y/N]: ")

	var response string
	fmt.Scanln(&response)

	response = fmt.Sprintf("%s", response)
	return response == "y" || response == "Y" || response == "yes" || response == "YES"
}

func maskKey(key string) string {
	if len(key) <= 8 {
		return "***"
	}
	return key[:4] + "***" + key[len(key)-4:]
}
