package messages

import (
	"bytes"
	"encoding/json"
	"fmt"

	"progressdb/pkg/logger"
	"progressdb/pkg/models"
	"progressdb/pkg/store/encryption"

	storedb "progressdb/pkg/store/db/store"
	"progressdb/pkg/store/keys"
	"progressdb/pkg/store/threads"
	"progressdb/pkg/telemetry"

	"github.com/cockroachdb/pebble"
)

// lists all messages for thread, ordered by insertion
func ListMessages(threadID string, limit ...int) ([]string, error) {
	tr := telemetry.Track("storedb.list_messages")
	defer tr.Finish()

	if storedb.Client == nil {
		return nil, fmt.Errorf("pebble not opened; call storedb.Open first")
	}

	// get thread for decryption
	threadStr, err := threads.GetThread(threadID)
	if err != nil {
		return nil, err
	}
	var thread models.Thread
	if err := json.Unmarshal([]byte(threadStr), &thread); err != nil {
		return nil, err
	}

	mp, merr := keys.MsgPrefix(threadID)
	if merr != nil {
		return nil, merr
	}
	prefix := []byte(mp)
	iter, err := storedb.Client.NewIter(&pebble.IterOptions{})
	if err != nil {
		return nil, err
	}
	defer iter.Close()
	var out []string
	max := -1
	if len(limit) > 0 {
		max = limit[0]
	}
	for iter.SeekGE(prefix); iter.Valid(); iter.Next() {
		if !bytes.HasPrefix(iter.Key(), prefix) {
			break
		}
		v := append([]byte(nil), iter.Value()...)
		// decrypt if enabled
		v, err = encryption.DecryptMessageData(&thread, v)
		if err != nil {
			logger.Error("decrypt_message_failed", "threadID", threadID, "error", err)
			return nil, fmt.Errorf("failed to decrypt message: %w", err)
		}
		out = append(out, string(v))
		if max > 0 && len(out) >= max {
			break
		}
	}
	return out, iter.Error()
}
