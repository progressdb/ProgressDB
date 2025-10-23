package messages

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

// deletes message and all version keys
func PurgeMessagePermanently(messageID string) error {
	// check stores are open
	if storedb.Client == nil {
		return fmt.Errorf("pebble not opened; call storedb.Open first")
	}
	if index.IndexDB == nil {
		return fmt.Errorf("pebble not opened; call Open first")
	}
	vprefix := keys.GenAllMessageVersionsPrefix(messageID)
	vi, err := index.IndexDB.NewIter(&pebble.IterOptions{})
	if err != nil {
		return err
	}
	defer vi.Close()

	// find metadata from first version
	var threadID, author string
	var seq int64
	var versionKeys [][]byte
	found := false
	for vi.SeekGE([]byte(vprefix)); vi.Valid(); vi.Next() {
		if !bytes.HasPrefix(vi.Key(), []byte(vprefix)) {
			break
		}
		if !found {
			// parse version key using proper parser
			if s, err := keys.ParseVersionKeySequence(string(vi.Key())); err == nil {
				seq = int64(s)
			}
			// unmarshal data to get thread and author
			v := append([]byte(nil), vi.Value()...)
			var msg models.Message
			if err := json.Unmarshal(v, &msg); err == nil {
				threadID = msg.Thread
				author = msg.Author
				found = true
			}
		}
		// collect all version keys
		versionKeys = append(versionKeys, append([]byte(nil), vi.Key()...))
	}

	// delete main message from main DB
	if found && threadID != "" {
		msgKey := keys.GenMessageKey(threadID, "", uint64(seq))
		if err := storedb.Client.Delete([]byte(msgKey), storedb.WriteOpt(true)); err != nil {
			logger.Error("purge_main_message_failed", "key", msgKey, "error", err)
		}
		// remove from deleted messages index
		if author != "" {
			if err := index.UpdateDeletedMessages(author, messageID, false); err != nil {
				logger.Error("update_deleted_messages_purge_failed", "user", author, "msg", messageID, "error", err)
			}
		}
	}

	// delete version keys from index DB
	for _, k := range versionKeys {
		if err := index.IndexDB.Delete(k, index.WriteOpt(true)); err != nil {
			logger.Error("purge_version_delete_failed", "key", string(k), "error", err)
		}
	}

	// log completion
	logger.Info("purge_message_completed", "msg", messageID, "deleted_keys", len(versionKeys))
	return nil
}
