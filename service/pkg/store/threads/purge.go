package store

import (
	"bytes"
	"encoding/json"
	"fmt"

	"progressdb/pkg/logger"
	"progressdb/pkg/models"

	"github.com/cockroachdb/pebble"
)

// deletes thread and all messages/versions; removes in batches
func PurgeThreadPermanently(threadID string) error {
	if db == nil {
		return fmt.Errorf("pebble not opened; call store.Open first")
	}
	tp, terr := ThreadPrefix(threadID)
	if terr != nil {
		return terr
	}
	prefix := []byte(tp)
	iter, err := db.NewIter(&pebble.IterOptions{})
	if err != nil {
		return err
	}
	defer iter.Close()
	const deleteBatchSize = 1000
	var batch [][]byte
	deleteBatch := func(keys [][]byte) {
		for _, k := range keys {
			if err := db.Delete(k, writeOpt(true)); err != nil {
				logger.Error("purge_delete_failed", "key", string(k), "error", err)
			}
		}
	}

	for iter.SeekGE(prefix); iter.Valid(); iter.Next() {
		if !bytes.HasPrefix(iter.Key(), prefix) {
			break
		}
		k := append([]byte(nil), iter.Key()...)
		batch = append(batch, k)
		v := append([]byte(nil), iter.Value()...)
		var m models.Message
		if err := json.Unmarshal(v, &m); err == nil && m.ID != "" {
			vprefix := []byte("version:msg:" + m.ID + ":")
			vi, _ := db.NewIter(&pebble.IterOptions{})
			if vi != nil {
				for vi.SeekGE(vprefix); vi.Valid(); vi.Next() {
					if !bytes.HasPrefix(vi.Key(), vprefix) {
						break
					}
					kk := append([]byte(nil), vi.Key()...)
					batch = append(batch, kk)
					if len(batch) >= deleteBatchSize {
						deleteBatch(batch)
						batch = batch[:0]
					}
				}
				vi.Close()
			}
		}
		if len(batch) >= deleteBatchSize {
			deleteBatch(batch)
			batch = batch[:0]
		}
	}
	if len(batch) > 0 {
		deleteBatch(batch)
	}
	_ = DeleteThread(threadID)
	logger.Info("purge_thread_completed", "thread", threadID, "deleted_keys", 0)
	return nil
}
