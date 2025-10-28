package messages

import (
	"bytes"
	"encoding/json"
	"fmt"

	"progressdb/pkg/models"
	"progressdb/pkg/state/logger"
	"progressdb/pkg/store/db/index"
	storedb "progressdb/pkg/store/db/store"
	"progressdb/pkg/store/keys"

	"github.com/cockroachdb/pebble"
)

func PurgeMessagePermanently(messageID string) error {
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

	var threadID string
	var seq int64
	var versionKeys [][]byte
	found := false
	for vi.SeekGE([]byte(vprefix)); vi.Valid(); vi.Next() {
		if !bytes.HasPrefix(vi.Key(), []byte(vprefix)) {
			break
		}
		if !found {
			if s, err := keys.ParseVersionKeySequence(string(vi.Key())); err == nil {
				seq = int64(s)
			}
			v := append([]byte(nil), vi.Value()...)
			var msg models.Message
			if err := json.Unmarshal(v, &msg); err == nil {
				threadID = msg.Thread
				found = true
			}
		}
		versionKeys = append(versionKeys, append([]byte(nil), vi.Key()...))
	}

	if found && threadID != "" {
		msgKey := keys.GenMessageKey(threadID, "", uint64(seq))
		if err := storedb.Client.Delete([]byte(msgKey), storedb.WriteOpt(true)); err != nil {
			logger.Error("purge_main_message_failed", "key", msgKey, "error", err)
		}
		if err := index.UnmarkSoftDeleted(messageID); err != nil {
			logger.Error("unmark_soft_deleted_purge_failed", "msg", messageID, "error", err)
		}
	}

	for _, k := range versionKeys {
		if err := index.IndexDB.Delete(k, index.WriteOpt(true)); err != nil {
			logger.Error("purge_version_delete_failed", "key", string(k), "error", err)
		}
	}

	logger.Info("purge_message_completed", "msg", messageID, "deleted_keys", len(versionKeys))
	return nil
}
