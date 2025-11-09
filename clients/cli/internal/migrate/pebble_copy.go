package migrate

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/cockroachdb/pebble"
	"progressdb/clients/cli/config"
)

// CopyPebbleDatabase copies a pebble database from source to target
func CopyPebbleDatabase(ctx context.Context, cfg *config.Config, verbose bool) error {
	if verbose {
		fmt.Printf("📋 Copying pebble database from %s to %s\n", cfg.FromDatabase, cfg.ToDatabase)
	}

	// Ensure target directory exists
	if err := os.MkdirAll(cfg.ToDatabase, 0755); err != nil {
		return fmt.Errorf("failed to create target directory: %w", err)
	}

	// Open source database (read-only)
	sourceDB, err := pebble.Open(cfg.FromDatabase, &pebble.Options{
		ReadOnly: true,
	})
	if err != nil {
		return fmt.Errorf("failed to open source database: %w", err)
	}
	defer sourceDB.Close()

	// Open target database (writable)
	targetDB, err := pebble.Open(cfg.ToDatabase, &pebble.Options{})
	if err != nil {
		return fmt.Errorf("failed to open target database: %w", err)
	}
	defer targetDB.Close()

	// Create snapshot for consistent copy
	snapshot := sourceDB.NewSnapshot()
	defer snapshot.Close()

	// Iterate through all keys and copy
	iter, _ := snapshot.NewIter(nil)
	defer iter.Close()

	count := 0
	batch := targetDB.NewBatch()
	defer batch.Close()

	for iter.First(); iter.Valid(); iter.Next() {
		key := iter.Key()
		value := iter.Value()

		// Copy key-value pair
		if err := batch.Set(key, value, nil); err != nil {
			return fmt.Errorf("failed to copy key %s: %w", string(key), err)
		}

		count++

		// Commit batch every 1000 entries to avoid large batches
		if count%1000 == 0 {
			if err := batch.Commit(nil); err != nil {
				return fmt.Errorf("failed to commit batch at count %d: %w", count, err)
			}
			batch = targetDB.NewBatch()
			defer batch.Close()

			if verbose {
				fmt.Printf("  Copied %d entries...\n", count)
			}
		}

		// Check for context cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
	}

	// Commit remaining entries
	if err := batch.Commit(nil); err != nil {
		return fmt.Errorf("failed to commit final batch: %w", err)
	}

	if verbose {
		fmt.Printf("✅ Successfully copied %d database entries\n", count)
	}

	return nil
}

// CopyPebbleFiles copies pebble database files directly
func CopyPebbleFiles(cfg *config.Config, targetDir string, verbose bool) error {
	if verbose {
		fmt.Printf("📋 Copying pebble files from %s to %s\n", cfg.FromDatabase, targetDir)
	}

	// Ensure target directory exists
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return fmt.Errorf("failed to create target directory: %w", err)
	}

	// Read source directory
	entries, err := os.ReadDir(cfg.FromDatabase)
	if err != nil {
		return fmt.Errorf("failed to read source directory: %w", err)
	}

	count := 0
	for _, entry := range entries {
		srcPath := filepath.Join(cfg.FromDatabase, entry.Name())
		dstPath := filepath.Join(targetDir, entry.Name())

		// Skip directories that shouldn't be copied directly
		if entry.IsDir() && (entry.Name() == "store" || entry.Name() == "wal" || entry.Name() == "kms" || entry.Name() == "state" || entry.Name() == "index") {
			if verbose {
				fmt.Printf("  Skipping directory: %s\n", entry.Name())
			}
			continue
		}

		if entry.IsDir() {
			// Recursively copy directory
			if err := copyDir(srcPath, dstPath, verbose); err != nil {
				return fmt.Errorf("failed to copy directory %s: %w", entry.Name(), err)
			}
		} else {
			// Copy file
			if err := copyFile(srcPath, dstPath); err != nil {
				return fmt.Errorf("failed to copy file %s: %w", entry.Name(), err)
			}
		}

		count++
		if verbose && count%10 == 0 {
			fmt.Printf("  Copied %d entries...\n", count)
		}
	}

	if verbose {
		fmt.Printf("✅ Successfully copied %d files/directories\n", count)
	}

	return nil
}

func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	return err
}

func copyDir(src, dst string, verbose bool) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dst, 0755); err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			if err := copyDir(srcPath, dstPath, verbose); err != nil {
				return err
			}
		} else {
			if err := copyFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}

	return nil
}
