package messages

import (
	"bytes"
	"fmt"

	"progressdb/pkg/logger"
	"progressdb/pkg/store/db/index"
	storedb "progressdb/pkg/store/db/store"

	"github.com/cockroachdb/pebble"
)

// deletes message and all version keys
func PurgeMessagePermanently(messageID string) error {
	if storedb.Client == nil {
		return fmt.Errorf("pebble not opened; call storedb.Open first")
	}
	// delete versions from index DB
	vprefix := []byte("idx:versions:" + messageID + ":")
	vi, err := index.IndexDB.NewIter(&pebble.IterOptions{})
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
		if err := index.IndexDB.Delete(k, index.IndexWriteOpt(true)); err != nil {
			logger.Error("purge_message_delete_failed", "key", string(k), "error", err)
		}
	}
	logger.Info("purge_message_completed", "msg", messageID, "deleted_keys", len(keys))
	return nil
}
