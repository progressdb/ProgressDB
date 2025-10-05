package progressor

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"progressdb/pkg/logger"
	"progressdb/pkg/models"
	"progressdb/pkg/store"
)

const (
	systemVersionKey    = "system:version"
	systemInProgressKey = "system:migration_in_progress"
)

// Sync performs upgrade work between versions. Edit in-place for migration logic.
func Sync(ctx context.Context, from, to string) error {
	logger.Info("progressor_sync_start", "from", from, "to", to)

	// Migration: initialize LastSeq for threads that lack it by scanning
	// existing message keys and setting thread.LastSeq to the highest
	// observed sequence. This is idempotent and safe to run multiple times.
	vals, err := store.ListThreads()
	if err != nil {
		logger.Error("progressor_list_threads_failed", "error", err)
		return err
	}
	for _, s := range vals {
		var th models.Thread
		if err := json.Unmarshal([]byte(s), &th); err != nil {
			logger.Error("progressor_unmarshal_thread_failed", "error", err)
			continue
		}
		if th.LastSeq != 0 {
			continue
		}
		// compute max seq for this thread
		max, err := store.MaxSeqForThread(th.ID)
		if err != nil {
			logger.Error("progressor_maxseq_failed", "thread", th.ID, "error", err)
			continue
		}
		if max == 0 {
			// nothing to do
			continue
		}
		th.LastSeq = max
		th.UpdatedTS = time.Now().UTC().UnixNano()
		nb, _ := json.Marshal(th)
		if err := store.SaveThread(th.ID, string(nb)); err != nil {
			logger.Error("progressor_save_thread_failed", "thread", th.ID, "error", err)
			continue
		}
		logger.Info("progressor_thread_lastseq_initialized", "thread", th.ID, "last_seq", max)
	}

	logger.Info("progressor_sync_done", "from", from, "to", to)
	return nil
}

// Run checks for a version change and runs Sync if needed.
// Returns (invoked, error): invoked is true if Sync ran.
func Run(ctx context.Context, newVersion string) (bool, error) {
	logger.Info("progressor_version_check", "stored", getStoredVersion(), "running", newVersion)

	stored, err := store.GetKey(systemVersionKey)
	if err != nil && err.Error() != "" {
		logger.Error("progressor_read_version_failed", "error", err)
	}
	if stored == newVersion {
		logger.Info("progressor_noop", "version", newVersion)
		return false, nil
	}

	marker := map[string]string{
		"from":       stored,
		"to":         newVersion,
		"started_at": time.Now().UTC().Format(time.RFC3339),
	}
	mb, _ := json.Marshal(marker)
	if err := store.SaveKey(systemInProgressKey, mb); err != nil {
		logger.Error("progressor_write_inprogress_failed", "error", err)
		return true, fmt.Errorf("failed to write in-progress marker: %w", err)
	}

	// TODO: add backup step here before performing migrations

	logger.Info("progressor_start_sync", "from", stored, "to", newVersion)
	if err := Sync(ctx, stored, newVersion); err != nil {
		logger.Error("progressor_sync_failed", "from", stored, "to", newVersion, "error", err)
		return true, err
	}
	logger.Info("progressor_sync_succeeded", "from", stored, "to", newVersion)

	if err := store.SaveKey(systemVersionKey, []byte(newVersion)); err != nil {
		logger.Error("progressor_persist_version_failed", "version", newVersion, "error", err)
		return true, fmt.Errorf("failed to persist new version: %w", err)
	}

	if err := store.DeleteKey(systemInProgressKey); err != nil {
		logger.Error("progressor_delete_inprogress_failed", "error", err)
	}

	logger.Info("progressor_version_persisted", "version", newVersion)
	return true, nil
}

func getStoredVersion() string {
	v, _ := store.GetKey(systemVersionKey)
	return v
}
