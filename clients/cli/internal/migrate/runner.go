package migrate

import (
	"fmt"

	"progressdb/clients/cli/config"
)

// Run executes the migration process
func Run(cfg *config.Config, verbose bool) error {
	if verbose {
		fmt.Println("Starting migration...")
		fmt.Printf("Source: %s\n", cfg.FromDatabase)
		fmt.Printf("Target: %s\n", cfg.ToDatabase)
		fmt.Printf("Format: %s\n", cfg.OutputFormat)
		fmt.Println()
	}

	// Route to appropriate migration function based on output format
	switch cfg.OutputFormat {
	case "pebble":
		if verbose {
			fmt.Println("Step 1: Migrating data format from 0.1.2 to 0.5.0 (Pebble)...")
		}
		if err := MigrateToPebble(cfg, verbose); err != nil {
			return fmt.Errorf("failed to migrate data format: %w", err)
		}
	case "json":
		fallthrough
	default:
		if verbose {
			fmt.Println("Step 1: Migrating data format from 0.1.2 to 0.5.0 (JSON)...")
		}
		if err := MigrateToJSON(cfg, verbose); err != nil {
			return fmt.Errorf("failed to migrate data format: %w", err)
		}
	}

	if verbose {
		fmt.Println("Migration completed successfully!")
	}

	return nil
}
