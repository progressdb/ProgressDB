package threads

import (
	"bytes"
	"encoding/json"
	"fmt"

	"progressdb/pkg/logger"
	"progressdb/pkg/models"
	"progressdb/pkg/store/db/index"
	storedb "progressdb/pkg/store/db/store"
	"progressdb/pkg/store/keys"

	"github.com/cockroachdb/pebble"
)

// deletes thread and all messages/versions; removes in batches
func PurgeThreadPermanently(threadID string) error {
	if storedb.Client == nil {
		return fmt.Errorf("pebble not opened; call storedb.Open first")
	}

	// Get thread to extract author
	sth, err := GetThread(threadID)
	if err != nil {
		return fmt.Errorf("failed to get thread: %w", err)
	}
	var th models.Thread
	if err := json.Unmarshal([]byte(sth), &th); err != nil {
		return fmt.Errorf("failed to unmarshal thread: %w", err)
	}

	tp, terr := keys.ThreadPrefix(threadID)
	if terr != nil {
		return terr
	}
	prefix := []byte(tp)
	iter, err := storedb.Client.NewIter(&pebble.IterOptions{})
	if err != nil {
		return err
	}
	defer iter.Close()
	const deleteBatchSize = 1000
	var mainBatch [][]byte
	var indexBatch [][]byte
	deleteMainBatch := func(keys [][]byte) {
		for _, k := range keys {
			if err := storedb.Client.Delete(k, storedb.WriteOpt(true)); err != nil {
				logger.Error("purge_delete_failed", "key", string(k), "error", err)
			}
		}
	}
	deleteIndexBatch := func(keys [][]byte) {
		for _, k := range keys {
			if err := index.IndexDB.Delete(k, index.WriteOpt(true)); err != nil {
				logger.Error("purge_index_delete_failed", "key", string(k), "error", err)
			}
		}
	}

	for iter.SeekGE(prefix); iter.Valid(); iter.Next() {
		if !bytes.HasPrefix(iter.Key(), prefix) {
			break
		}
		k := append([]byte(nil), iter.Key()...)
		mainBatch = append(mainBatch, k)
		v := append([]byte(nil), iter.Value()...)
		var m models.Message
		if err := json.Unmarshal(v, &m); err == nil && m.ID != "" {
			// delete versions from index DB
			vprefix := []byte("idx:versions:" + m.ID + ":")
			vi, _ := index.IndexDB.NewIter(&pebble.IterOptions{})
			if vi != nil {
				for vi.SeekGE(vprefix); vi.Valid(); vi.Next() {
					if !bytes.HasPrefix(vi.Key(), vprefix) {
						break
					}
					kk := append([]byte(nil), vi.Key()...)
					indexBatch = append(indexBatch, kk)
					if len(indexBatch) >= deleteBatchSize {
						deleteIndexBatch(indexBatch)
						indexBatch = indexBatch[:0]
					}
				}
				vi.Close()
			}
		}
		if len(mainBatch) >= deleteBatchSize {
			deleteMainBatch(mainBatch)
			mainBatch = mainBatch[:0]
		}
	}
	if len(mainBatch) > 0 {
		deleteMainBatch(mainBatch)
	}
	if len(indexBatch) > 0 {
		deleteIndexBatch(indexBatch)
	}

	// Delete thread message indexes
	if err := index.DeleteThreadMessageIndexes(threadID); err != nil {
		logger.Error("delete_thread_message_indexes_failed", "thread", threadID, "error", err)
		// Continue with purge
	}

	_ = DeleteThread(threadID)

	// Delete user thread indexes
	if err := index.DeleteUserThreadIndexes(th.Author); err != nil {
		logger.Error("delete_user_thread_indexes_failed", "user", th.Author, "error", err)
		// Continue
	}

	// Delete thread participant indexes
	if err := index.DeleteThreadParticipantIndexes(threadID); err != nil {
		logger.Error("delete_thread_participant_indexes_failed", "thread", threadID, "error", err)
		// Continue
	}

	logger.Info("purge_thread_completed", "thread", threadID, "deleted_keys", 0)
	return nil
}
