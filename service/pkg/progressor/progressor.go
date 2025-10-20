package progressor

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"progressdb/pkg/logger"
	"progressdb/pkg/models"
	"progressdb/pkg/store/threads"
	"progressdb/pkg/store/db"
)

const (
	systemVersionKey    = "system:version"
	systemInProgressKey = "system:migration_in_progress"
)

const defaultStoredVersion = "0.1.2"

// migrateTo0_2_0 initializes per-thread LastSeq when missing (idempotent).
func migrateTo0_2_0(ctx context.Context, from, to string) error {
	logger.Info("migration_start", "from", from, "to", to)

	// Give all threads the new last seq field
	vals, err := threads.ListThreads()
	if err != nil {
		logger.Error("migration_list_threads_failed", "error", err)
		return err
	}

	for _, s := range vals {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		var th models.Thread
		if err := json.Unmarshal([]byte(s), &th); err != nil {
			logger.Error("migration_unmarshal_thread_failed", "error", err)
			continue
		}
		if th.LastSeq != 0 {
			continue
		}

		max, err := threads.MaxSeqForThread(th.ID)
		if err != nil {
			logger.Error("migration_maxseq_failed", "thread", th.ID, "error", err)
			continue
		}
		if max == 0 {
			continue
		}

		th.LastSeq = max
		th.UpdatedTS = time.Now().UTC().UnixNano()
		nb, _ := json.Marshal(th)
		if err := threads.SaveThread(th.ID, string(nb)); err != nil {
			logger.Error("migration_save_thread_failed", "thread", th.ID, "error", err)
			return err
		}
		logger.Info("migration_thread_lastseq_initialized", "thread", th.ID, "last_seq", max)
	}

	logger.Info("migration_done", "to", to)
	return nil
}

// startMigration writes the in-progress marker and logs the start of a migration.
func startMigration(from, to string) error {
	marker := map[string]string{"from": from, "to": to, "started_at": time.Now().UTC().Format(time.RFC3339)}
	mb, _ := json.Marshal(marker)
	if err := db.SaveKey(systemInProgressKey, mb); err != nil {
		logger.Error("progressor_write_inprogress_failed", "error", err)
		return fmt.Errorf("failed to write in-progress marker: %w", err)
	}
	logger.Info("progressor_start_sync", "from", from, "to", to)
	return nil
}

// finishMigration persists the new version, clears the in-progress marker and logs.
func finishMigration(to string) error {
	if err := db.SaveKey(systemVersionKey, []byte(to)); err != nil {
		logger.Error("progressor_persist_version_failed", "version", to, "error", err)
		return fmt.Errorf("failed to persist new version: %w", err)
	}
	if err := db.DeleteKey(systemInProgressKey); err != nil {
		logger.Error("progressor_delete_inprogress_failed", "error", err)
	}
	logger.Info("progressor_version_persisted", "version", to)
	return nil
}

// Migrations are self-contained; Run dispatches the matching handler.
func Run(ctx context.Context, newVersion string) (bool, error) {
	stored, err := db.GetKey(systemVersionKey)
	if err != nil {
		// treat missing system version key as a first-run case and use the
		// historical last-known version as a sensible default so migrations
		// are calculated from that point.
		if db.IsNotFound(err) {
			stored = defaultStoredVersion
		} else {
			logger.Error("progressor_read_version_failed", "error", err)
		}
	}
	if stored == newVersion {
		return false, nil
	}

	// Dispatch known migrations; unknown versions are persisted silently.
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
		if err := db.SaveKey(systemVersionKey, []byte(newVersion)); err != nil {
			logger.Error("progressor_persist_version_failed", "version", newVersion, "error", err)
			return true, fmt.Errorf("failed to persist new version: %w", err)
		}
		return true, nil
	}
}

func getStoredVersion() string {
	v, err := db.GetKey(systemVersionKey)
	if err != nil {
		if db.IsNotFound(err) {
			return defaultStoredVersion
		}
		return ""
	}
	return v
}
