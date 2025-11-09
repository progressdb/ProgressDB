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
  progressdb migrate --old-config /old/config.yaml --to /new/db
  progressdb migrate --old-db-path /old/db --old-encryption-key <key> --to /new/db
  progressdb migrate --from /old/db --to /new/db --config config.yaml
  progressdb migrate --from /old/db --to /new/db  # will prompt for missing info

Configuration options:
  --old-config:        Load all settings from an existing 0.1.2 service config
  --old-db-path:        Specify old database path directly
  --old-encryption-key: Specify old encryption key directly (hex, 32 bytes)
  --from:               Source database path (alternative to --old-db-path)
  --to:                 Target database path (required)
  --config:             Migration configuration file
  --format:             Output format: json or pebble (default: json)`,
	RunE: runMigrate,
}

var (
	fromPath         string
	toPath           string
	configFile       string
	oldConfigFile    string
	oldDBPath        string
	oldEncryptionKey string
	interactive      bool
	outputFormat     string
)

func init() {
	rootCmd.AddCommand(migrateCmd)

	migrateCmd.Flags().StringVar(&fromPath, "from", "", "path to source database (optional if --old-config or --old-db-path is provided)")
	migrateCmd.Flags().StringVar(&toPath, "to", "", "path to target database (required)")
	migrateCmd.Flags().StringVar(&configFile, "config", "", "migration configuration file path")
	migrateCmd.Flags().StringVar(&oldConfigFile, "old-config", "", "old service configuration file path (0.1.2)")
	migrateCmd.Flags().StringVar(&oldDBPath, "old-db-path", "", "old database path (alternative to --from)")
	migrateCmd.Flags().StringVar(&oldEncryptionKey, "old-encryption-key", "", "old encryption key (hex, 32 bytes)")
	migrateCmd.Flags().StringVar(&outputFormat, "format", "json", "output format: json or pebble (default: json)")
	migrateCmd.Flags().BoolVar(&interactive, "interactive", true, "enable interactive prompts for missing values")

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

	// Load old configuration if provided
	if oldConfigFile != "" {
		if err := cfg.LoadOldConfig(oldConfigFile); err != nil {
			return nil, fmt.Errorf("failed to load old config file: %w", err)
		}
	}

	// Override with command line flags
	if fromPath != "" {
		cfg.FromDatabase = fromPath
	}
	if oldDBPath != "" {
		cfg.FromDatabase = oldDBPath
	}
	if oldEncryptionKey != "" {
		cfg.OldEncryptionKey = oldEncryptionKey
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
		return fmt.Errorf("source database path is required (provide --from or --old-config)")
	}
	if cfg.ToDatabase == "" {
		return fmt.Errorf("target database path is required")
	}
	if cfg.OldEncryptionKey == "" {
		return fmt.Errorf("old encryption key is required for migration (provide in config or --old-config)")
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

	if len(cfg.OldEncryptFields) > 0 {
		fmt.Printf("  Encrypted Fields: %d\n", len(cfg.OldEncryptFields))
		if verbose {
			for _, field := range cfg.OldEncryptFields {
				fmt.Printf("    - %s (%s)\n", field.Path, field.Algorithm)
			}
		}
	}

	if verbose {
		if configFile != "" {
			fmt.Printf("  Config File: %s\n", configFile)
		}
		if oldConfigFile != "" {
			fmt.Printf("  Old Config:  %s\n", oldConfigFile)
		}
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
