package migrations

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"progressdb/pkg/config"
	"progressdb/pkg/state"
	"progressdb/pkg/state/logger"
	indexdb "progressdb/pkg/store/db/indexdb"
	storedb "progressdb/pkg/store/db/storedb"
	"progressdb/pkg/store/keys"
	"progressdb/pkg/timeutil"
)

const defaultStoredVersion = "0.1.2"

func startMigration(from, to string) error {
	marker := map[string]string{
		"from":       from,
		"to":         to,
		"started_at": timeutil.Now().Format(time.RFC3339),
	}
	mb, _ := json.Marshal(marker)
	if err := storedb.SaveKey(keys.SystemInProgressKey, mb); err != nil {
		logger.Error("[MIGRATIONS] progressor_write_inprogress_failed", "error", err)
		return fmt.Errorf("failed to write in-progress marker: %w", err)
	}
	logger.Info("[MIGRATIONS] progressor_start_sync", "from", from, "to", to)
	return nil
}

func finishMigration(to string) error {
	if err := storedb.SaveKey(keys.SystemVersionKey, []byte(to)); err != nil {
		logger.Error("[MIGRATIONS] progressor_persist_version_failed", "version", to, "error", err)
		return fmt.Errorf("failed to persist new version: %w", err)
	}
	if err := storedb.DeleteKey(keys.SystemInProgressKey); err != nil {
		logger.Error("[MIGRATIONS] progressor_delete_inprogress_failed", "error", err)
	}
	logger.Info("[MIGRATIONS] progressor_version_persisted", "version", to)
	return nil
}

func Run(ctx context.Context, newVersion string) (bool, error) {
	logger.Info("[MIGRATIONS] migration_run_invoked", "new_version", newVersion)
	stored, err := storedb.GetKey(keys.SystemVersionKey)
	if err != nil {
		if storedb.IsNotFound(err) {
			// For fresh installs, skip to current version
			stored = newVersion
			logger.Info("[MIGRATIONS] migration_version_not_found_defaulting_to_current", "default", stored)
		} else {
			logger.Error("[MIGRATIONS] progressor_read_version_failed", "error", err)
			return false, err
		}
	}
	logger.Info("[MIGRATIONS] migration_current_stored_key", "stored_value", stored)

	// skip if same version already
	if stored == newVersion {
		logger.Info("[MIGRATIONS] migration_version_up_to_date", "version", newVersion)
		return false, nil
	}
	logger.Info("[MIGRATIONS] migration_version_upgrade_required", "from", stored, "to", newVersion)

	// migrate
	if stored == "0.1.2" && (newVersion == "0.2.0" || newVersion == "v0.2.0") {
		// check if a 0.1.2 db exist
		cfg := config.GetConfig()
		if cfg != nil {
			if _, err := os.Stat(cfg.Server.DBPath); os.IsNotExist(err) {
				logger.Info("[MIGRATIONS] old database does not exist, skipping migration", "path", cfg.Server.DBPath)
				// skip to the new version - no db files exist
				if err := finishMigration(newVersion); err != nil {
					logger.Error("[MIGRATIONS] failed to set current version", "error", err)
					return true, err
				}
				return true, nil
			}
		}

		if err := startMigration(stored, newVersion); err != nil {
			return true, err
		}

		if herr := bumpTo0_5_0(ctx); herr != nil {
			logger.Error("[MIGRATIONS] progressor_migration_handler_failed", "from", stored, "to", newVersion, "error", herr)
			return true, herr
		}

		if err := backupOldDatabaseFiles(stored); err != nil {
			logger.Error("[MIGRATIONS] backup_old_database_failed", "error", err)
			return true, err
		}

		if err := finishMigration(newVersion); err != nil {
			return true, err
		}
		logger.Info("[MIGRATIONS] migration_completed", "from", stored, "to", newVersion)
		return true, nil
	}

	if err := storedb.SaveKey(keys.SystemVersionKey, []byte(newVersion)); err != nil {
		logger.Error("[MIGRATIONS] progressor_persist_version_failed", "version", newVersion, "error", err)
		return true, fmt.Errorf("failed to persist new version: %w", err)
	}
	logger.Info("[MIGRATIONS] migration_version_set_direct", "new_version", newVersion)
	return true, nil
}

func bumpTo0_5_0(ctx context.Context) error {
	logger.Info("[MIGRATIONS] Starting bump to 0.2.0")

	if storedb.Client == nil {
		return fmt.Errorf("store database client not initialized")
	}
	if indexdb.Client == nil {
		return fmt.Errorf("index database client not initialized")
	}

	if err := MigrateToStore(ctx, storedb.Client, indexdb.Client); err != nil {
		return fmt.Errorf("failed to migrate to store databases: %w", err)
	}

	logger.Info("[MIGRATIONS] Bump to 0.5.0 completed successfully")
	return nil
}

func backupOldDatabaseFiles(fromVersion string) error {
	cfg := config.GetConfig()
	if cfg == nil {
		return fmt.Errorf("service configuration not available")
	}

	sourcePath := cfg.Server.DBPath
	if sourcePath == "" {
		return fmt.Errorf("database path not configured")
	}

	// Convert version to filesystem-safe format (replace dots with underscores)
	cleanVersion := strings.ReplaceAll(fromVersion, ".", "_")
	backupPath := filepath.Join(state.PathsVar.Backups, "migration_backup_"+cleanVersion)
	if err := os.MkdirAll(backupPath, 0o700); err != nil {
		return fmt.Errorf("failed to create backup directory: %w", err)
	}

	entries, err := os.ReadDir(sourcePath)
	if err != nil {
		return fmt.Errorf("failed to read source directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue // skip directories (store, index, etc.)
		}

		srcPath := filepath.Join(sourcePath, entry.Name())
		dstPath := filepath.Join(backupPath, entry.Name())

		if err := os.Rename(srcPath, dstPath); err != nil {
			logger.Warn("[MIGRATIONS] backup_file_failed", "file", entry.Name(), "error", err)
			continue
		}

		logger.Info("[MIGRATIONS] backup_file_completed", "file", entry.Name(), "backup_path", backupPath)
	}

	logger.Info("[MIGRATIONS] backup_completed", "from_version", fromVersion, "backup_path", backupPath)
	return nil
}
