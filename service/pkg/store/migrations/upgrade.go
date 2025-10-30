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

func migrateTo0_2_0(ctx context.Context, from, to string) error {
	logger.Info("migrateTo0_2_0 called", "from", from, "to", to)
	// TODO: Implement rekeying, encryptions etc
	return nil
}

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

	switch newVersion {
	case "0.2.0":
		if err := startMigration(stored, newVersion); err != nil {
			return true, err
		}

		if herr := migrateTo0_2_0(ctx, stored, newVersion); herr != nil {
			logger.Error("progressor_migration_handler_failed", "from", stored, "to", newVersion, "error", herr)
			return true, herr
		}

		if err := finishMigration(newVersion); err != nil {
			return true, err
		}
		return true, nil
	default:
		if err := storedb.SaveKey(keys.SystemVersionKey, []byte(newVersion)); err != nil {
			logger.Error("progressor_persist_version_failed", "version", newVersion, "error", err)
			return true, fmt.Errorf("failed to persist new version: %w", err)
		}
		return true, nil
	}
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
