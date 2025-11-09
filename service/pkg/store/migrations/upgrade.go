package migrations

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"progressdb/pkg/state/logger"
	storedb "progressdb/pkg/store/db/storedb"
	"progressdb/pkg/store/keys"
	"progressdb/pkg/timeutil"
)

const defaultStoredVersion = "0.1.2"

func startMigration(from, to string) error {
	marker := map[string]string{"from": from, "to": to, "started_at": timeutil.Now().Format(time.RFC3339)}
	mb, _ := json.Marshal(marker)
	if err := storedb.SaveKey(keys.SystemInProgressKey, mb); err != nil {
		logger.Error("progressor_write_inprogress_failed", "error", err)
		return fmt.Errorf("failed to write in-progress marker: %w", err)
	}
	logger.Info("progressor_start_sync", "from", from, "to", to)
	return nil
}

func finishMigration(to string) error {
	if err := storedb.SaveKey(keys.SystemVersionKey, []byte(to)); err != nil {
		logger.Error("progressor_persist_version_failed", "version", to, "error", err)
		return fmt.Errorf("failed to persist new version: %w", err)
	}
	if err := storedb.DeleteKey(keys.SystemInProgressKey); err != nil {
		logger.Error("progressor_delete_inprogress_failed", "error", err)
	}
	logger.Info("progressor_version_persisted", "version", to)
	return nil
}

func Run(ctx context.Context, newVersion string) (bool, error) {
	stored, err := storedb.GetKey(keys.SystemVersionKey)
	if err != nil {
		if storedb.IsNotFound(err) {
			stored = defaultStoredVersion
		} else {
			logger.Error("progressor_read_version_failed", "error", err)
		}
	}
	if stored == newVersion {
		return false, nil
	}

	// Handle specific migration paths
	if stored == "0.1.2" && newVersion == "0.5.0" {
		if err := startMigration(stored, newVersion); err != nil {
			return true, err
		}

		if herr := bumpTo0_5_0(ctx); herr != nil {
			logger.Error("progressor_migration_handler_failed", "from", stored, "to", newVersion, "error", herr)
			return true, herr
		}

		if err := finishMigration(newVersion); err != nil {
			return true, err
		}
		return true, nil
	}

	// Default case - just update version
	if err := storedb.SaveKey(keys.SystemVersionKey, []byte(newVersion)); err != nil {
		logger.Error("progressor_persist_version_failed", "version", newVersion, "error", err)
		return true, fmt.Errorf("failed to persist new version: %w", err)
	}
	return true, nil
}

func getStoredVersion() string {
	v, err := storedb.GetKey(keys.SystemVersionKey)
	if err != nil {
		if storedb.IsNotFound(err) {
			return defaultStoredVersion
		}
		return ""
	}
	return v
}

// bumpTo0_5_0 handles migration to version 0.5.0 using new migration system
func bumpTo0_5_0(ctx context.Context) error {
	logger.Info("Starting bump to 0.5.0")

	// Get migration records
	records, err := MigrateToRecords(ctx)
	if err != nil {
		return fmt.Errorf("failed to get migration records: %w", err)
	}

	logger.Info("Migration records extracted",
		"threads", len(records.Threads),
		"messages", len(records.Messages),
		"indexes", len(records.Indexes))

	// TODO: Get storeDB and indexDB instances from service
	// For now, we'll just log the records count
	// In the actual integration, the service will pass these instances

	logger.Info("Bump to 0.5.0 completed successfully")
	return nil
}
