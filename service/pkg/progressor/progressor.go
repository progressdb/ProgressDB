package progressor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"progressdb/pkg/logger"
	"progressdb/pkg/models"
	"progressdb/pkg/store/db/index"
	storedb "progressdb/pkg/store/db/store"
	"progressdb/pkg/store/keys"
	"progressdb/pkg/timeutil"
)

const (
	systemVersionKey    = "system:version"
	systemInProgressKey = "system:migration_in_progress"
)

const defaultStoredVersion = "0.1.2"

// migrateTo0_2_0 initializes per-thread LastSeq when missing (idempotent).
func migrateTo0_2_0(ctx context.Context, from, to string) error {
	logger.Info("migration_start", "from", from, "to", to)

	// Iterate through all thread metadata keys
	threadPrefix := keys.GenThreadMetadataPrefix()
	iter, err := storedb.Client.NewIter(nil)
	if err != nil {
		logger.Error("migration_create_iterator_failed", "error", err)
		return err
	}
	defer iter.Close()

	for iter.SeekGE([]byte(threadPrefix)); iter.Valid(); iter.Next() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		key := iter.Key()
		// Check if this is a thread metadata key (ends with ":meta")
		if !bytes.HasSuffix(key, []byte(":meta")) {
			continue
		}

		threadID := string(key[:len(key)-5]) // Remove ":meta" suffix
		if threadID == "" {
			continue
		}

		var th models.Thread
		if err := json.Unmarshal(iter.Value(), &th); err != nil {
			logger.Error("migration_unmarshal_thread_failed", "thread", threadID, "error", err)
			continue
		}
		if th.LastSeq != 0 {
			continue
		}

		// Get the max sequence from thread message indexes
		threadIndexes, err := index.GetThreadMessageIndexes(threadID)
		if err != nil {
			logger.Error("migration_get_indexes_failed", "thread", threadID, "error", err)
			continue
		}

		max := threadIndexes.End
		if max == 0 {
			continue
		}

		th.LastSeq = max
		th.UpdatedTS = timeutil.Now().UnixNano()
		nb, _ := json.Marshal(th)
		threadKey := keys.GenThreadKey(threadID)
		if err := storedb.SaveKey(threadKey, nb); err != nil {
			logger.Error("migration_save_thread_failed", "thread", threadID, "error", err)
			return err
		}
		logger.Info("migration_thread_lastseq_initialized", "thread", threadID, "last_seq", max)
	}

	logger.Info("migration_done", "to", to)
	return nil
}

// startMigration writes the in-progress marker and logs the start of a migration.
func startMigration(from, to string) error {
	marker := map[string]string{"from": from, "to": to, "started_at": timeutil.Now().Format(time.RFC3339)}
	mb, _ := json.Marshal(marker)
	if err := storedb.SaveKey(systemInProgressKey, mb); err != nil {
		logger.Error("progressor_write_inprogress_failed", "error", err)
		return fmt.Errorf("failed to write in-progress marker: %w", err)
	}
	logger.Info("progressor_start_sync", "from", from, "to", to)
	return nil
}

// finishMigration persists the new version, clears the in-progress marker and logs.
func finishMigration(to string) error {
	if err := storedb.SaveKey(systemVersionKey, []byte(to)); err != nil {
		logger.Error("progressor_persist_version_failed", "version", to, "error", err)
		return fmt.Errorf("failed to persist new version: %w", err)
	}
	if err := storedb.DeleteKey(systemInProgressKey); err != nil {
		logger.Error("progressor_delete_inprogress_failed", "error", err)
	}
	logger.Info("progressor_version_persisted", "version", to)
	return nil
}

// Migrations are self-contained; Run dispatches the matching handler.
func Run(ctx context.Context, newVersion string) (bool, error) {
	stored, err := storedb.GetKey(systemVersionKey)
	if err != nil {
		// treat missing system version key as a first-run case and use the
		// historical last-known version as a sensible default so migrations
		// are calculated from that point.
		if storedb.IsNotFound(err) {
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
		if err := storedb.SaveKey(systemVersionKey, []byte(newVersion)); err != nil {
			logger.Error("progressor_persist_version_failed", "version", newVersion, "error", err)
			return true, fmt.Errorf("failed to persist new version: %w", err)
		}
		return true, nil
	}
}

func getStoredVersion() string {
	v, err := storedb.GetKey(systemVersionKey)
	if err != nil {
		if storedb.IsNotFound(err) {
			return defaultStoredVersion
		}
		return ""
	}
	return v
}
