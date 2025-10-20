package messages

import (
	"bytes"
	"fmt"

	"progressdb/pkg/logger"
	"progressdb/pkg/store/db"

	"github.com/cockroachdb/pebble"
)

// deletes message and all version keys
func PurgeMessagePermanently(messageID string) error {
	if db.StoreDB == nil {
		return fmt.Errorf("pebble not opened; call store.Open first")
	}
	vprefix := []byte("version:msg:" + messageID + ":")
	vi, err := db.StoreDB.NewIter(&pebble.IterOptions{})
	if err != nil {
		return err
	}
	defer vi.Close()
	var keys [][]byte
	for vi.SeekGE(vprefix); vi.Valid(); vi.Next() {
		if !bytes.HasPrefix(vi.Key(), vprefix) {
			break
		}
		keys = append(keys, append([]byte(nil), vi.Key()...))
	}
	for _, k := range keys {
		if err := db.StoreDB.Delete(k, db.WriteOpt(true)); err != nil {
			logger.Error("purge_message_delete_failed", "key", string(k), "error", err)
		}
	}
	logger.Info("purge_message_completed", "msg", messageID, "deleted_keys", len(keys))
	return nil
}
