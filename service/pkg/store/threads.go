package store

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"progressdb/pkg/logger"
	"progressdb/pkg/models"
	"progressdb/pkg/utils"

	"github.com/cockroachdb/pebble"
)

// saves thread metadata as JSON
func SaveThread(threadID, data string) error {
	if db == nil {
		return fmt.Errorf("pebble not opened; call store.Open first")
	}
	tk, err := ThreadMetaKey(threadID)
	if err != nil {
		return fmt.Errorf("invalid thread id: %w", err)
	}
	if err := db.Set([]byte(tk), []byte(data), writeOpt(true)); err != nil {
		logger.Error("save_thread_failed", "thread", threadID, "error", err)
		return err
	}
	logger.Info("thread_saved", "thread", threadID)
	return nil
}

// gets thread metadata JSON for id
func GetThread(threadID string) (string, error) {
	if db == nil {
		return "", fmt.Errorf("pebble not opened; call store.Open first")
	}
	tk, err := ThreadMetaKey(threadID)
	if err != nil {
		return "", fmt.Errorf("invalid thread id: %w", err)
	}
	v, closer, err := db.Get([]byte(tk))
	if err != nil {
		return "", err
	}
	if closer != nil {
		defer closer.Close()
	}
	return string(v), nil
}

// deletes thread metadata
func DeleteThread(threadID string) error {
	if db == nil {
		return fmt.Errorf("pebble not opened; call store.Open first")
	}
	tk, err := ThreadMetaKey(threadID)
	if err != nil {
		return fmt.Errorf("invalid thread id: %w", err)
	}
	if err := db.Delete([]byte(tk), writeOpt(true)); err != nil {
		logger.Error("delete_thread_failed", "thread", threadID, "error", err)
		return err
	}
	logger.Info("thread_deleted", "thread", threadID)
	return nil
}

// marks thread as deleted and adds a tombstone message
func SoftDeleteThread(threadID, actor string) error {
	if db == nil {
		return fmt.Errorf("pebble not opened; call store.Open first")
	}
	tk, terr := ThreadMetaKey(threadID)
	if terr != nil {
		return terr
	}
	key := []byte(tk)
	v, closer, err := db.Get(key)
	if err != nil {
		logger.Error("soft_delete_load_failed", "thread", threadID, "error", err)
		return err
	}
	if closer != nil {
		defer closer.Close()
	}
	var th models.Thread
	if err := json.Unmarshal(v, &th); err != nil {
		logger.Error("soft_delete_unmarshal_failed", "thread", threadID, "error", err)
		return err
	}
	th.Deleted = true
	th.DeletedTS = time.Now().UTC().UnixNano()
	nb, _ := json.Marshal(th)
	if err := db.Set(key, nb, writeOpt(true)); err != nil {
		logger.Error("soft_delete_save_failed", "thread", threadID, "error", err)
		return err
	}
	tomb := models.Message{
		ID:      utils.GenID(),
		Thread:  threadID,
		Author:  actor,
		TS:      time.Now().UTC().UnixNano(),
		Body:    map[string]interface{}{"_event": "thread_deleted", "by": actor},
		Deleted: true,
	}
	// use background context
	if err := SaveMessage(context.Background(), threadID, tomb.ID, tomb); err != nil {
		logger.Error("soft_delete_append_tombstone_failed", "thread", threadID, "error", err)
		return err
	}
	logger.Info("thread_soft_deleted", "thread", threadID, "actor", actor)
	return nil
}

// lists all saved thread metadata as JSON
func ListThreads() ([]string, error) {
	if db == nil {
		return nil, fmt.Errorf("pebble not opened; call store.Open first")
	}
	prefix := []byte("thread:")
	iter, err := db.NewIter(&pebble.IterOptions{})
	if err != nil {
		return nil, err
	}
	defer iter.Close()
	var out []string
	for iter.SeekGE(prefix); iter.Valid(); iter.Next() {
		if !bytes.HasPrefix(iter.Key(), prefix) {
			break
		}
		k := string(iter.Key())
		if strings.HasSuffix(k, ":meta") {
			v := append([]byte(nil), iter.Value()...)
			out = append(out, string(v))
		}
	}
	return out, iter.Error()
}

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
