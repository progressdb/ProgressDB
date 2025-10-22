package messages

import (
	"bytes"
	"encoding/base64"
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

// ListMessagesCursor lists messages for thread with cursor-based pagination
func ListMessagesCursor(threadID, cursor string, limit int) ([]string, string, bool, error) {
	tr := telemetry.Track("storedb.list_messages_cursor")
	defer tr.Finish()

	if storedb.Client == nil {
		return nil, "", false, fmt.Errorf("pebble not opened; call storedb.Open first")
	}

	// get thread for decryption
	threadStr, err := threads.GetThread(threadID)
	if err != nil {
		return nil, "", false, err
	}
	var thread models.Thread
	if err := json.Unmarshal([]byte(threadStr), &thread); err != nil {
		return nil, "", false, err
	}

	iter, err := storedb.Client.NewIter(&pebble.IterOptions{})
	if err != nil {
		return nil, "", false, err
	}
	defer iter.Close()

	var startKey []byte
	if cursor != "" {
		// Decode cursor to get starting position
		mc, err := decodeMessageCursor(cursor)
		if err != nil {
			return nil, "", false, fmt.Errorf("invalid cursor: %w", err)
		}
		if mc.ThreadID != threadID {
			return nil, "", false, fmt.Errorf("cursor thread mismatch")
		}
		// Create start key from cursor
		startKeyStr, err := keys.MsgKey(threadID, mc.Timestamp, mc.Sequence)
		if err != nil {
			return nil, "", false, err
		}
		startKey = []byte(startKeyStr)
		// We want to start AFTER the cursor position
		startKey = append(startKey, 0x00) // Ensure we seek past the exact key
	} else {
		// No cursor, start from beginning
		mp, merr := keys.MsgPrefix(threadID)
		if merr != nil {
			return nil, "", false, merr
		}
		startKey = []byte(mp)
	}

	var out []string
	var lastTimestamp int64
	var lastSequence uint64
	count := 0

	for iter.SeekGE(startKey); iter.Valid(); iter.Next() {
		key := iter.Key()

		// Check if we're still in the thread's message range
		mp, merr := keys.MsgPrefix(threadID)
		if merr != nil {
			return nil, "", false, merr
		}
		if !bytes.HasPrefix(key, []byte(mp)) {
			break
		}

		// Parse key to extract timestamp and sequence
		_, ts, seq, err := keys.ParseMsgKey(string(key))
		if err != nil {
			continue // Skip invalid keys
		}

		v := append([]byte(nil), iter.Value()...)
		// decrypt if enabled
		v, err = encryption.DecryptMessageData(thread.KMS, v)
		if err != nil {
			logger.Error("decrypt_message_failed", "threadID", threadID, "error", err)
			return nil, "", false, fmt.Errorf("failed to decrypt message: %w", err)
		}

		out = append(out, string(v))
		lastTimestamp = ts
		lastSequence = seq
		count++

		if count >= limit {
			break
		}
	}

	// Determine if there are more messages
	hasMore := iter.Valid() && bytes.HasPrefix(iter.Key(), startKey[:bytes.LastIndexByte(startKey, ':')+1])

	// Generate next cursor if we have messages and might have more
	var nextCursor string
	if len(out) > 0 && hasMore {
		nextCursor, err = encodeMessageCursor(threadID, lastTimestamp, lastSequence)
		if err != nil {
			return nil, "", false, err
		}
	}

	return out, nextCursor, hasMore, iter.Error()
}

// Helper functions for cursor encoding/decoding (moved from api package)
func encodeMessageCursor(threadID string, timestamp int64, sequence uint64) (string, error) {
	cursor := map[string]interface{}{
		"thread_id": threadID,
		"timestamp": timestamp,
		"sequence":  sequence,
	}
	data, err := json.Marshal(cursor)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(data), nil
}

func decodeMessageCursor(cursor string) (struct {
	ThreadID  string `json:"thread_id"`
	Timestamp int64  `json:"timestamp"`
	Sequence  uint64 `json:"sequence"`
}, error) {
	var result struct {
		ThreadID  string `json:"thread_id"`
		Timestamp int64  `json:"timestamp"`
		Sequence  uint64 `json:"sequence"`
	}

	data, err := base64.StdEncoding.DecodeString(cursor)
	if err != nil {
		return result, err
	}
	err = json.Unmarshal(data, &result)
	return result, err
}
