package messages

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"

	"progressdb/pkg/logger"
	"progressdb/pkg/models"
	"progressdb/pkg/store/db/index"
	"progressdb/pkg/store/encryption"
	"progressdb/pkg/store/features/threads"

	storedb "progressdb/pkg/store/db/store"
	"progressdb/pkg/store/keys"
	"progressdb/pkg/telemetry"

	"github.com/cockroachdb/pebble"
)

func ListMessages(threadID string, reqCursor models.ReadRequestCursorInfo) ([]string, models.ReadResponseCursorInfo, error) {
	tr := telemetry.Track("storedb.list_messages_cursor")
	defer tr.Finish()

	if storedb.Client == nil {
		return nil, models.ReadResponseCursorInfo{}, fmt.Errorf("pebble not opened; call storedb.Open first")
	}

	threadStr, err := threads.GetThread(threadID)
	if err != nil {
		return nil, models.ReadResponseCursorInfo{}, err
	}
	var thread models.Thread
	if err := json.Unmarshal([]byte(threadStr), &thread); err != nil {
		return nil, models.ReadResponseCursorInfo{}, err
	}

	threadIndexes, err := index.GetThreadMessageIndexes(threadID)
	if err != nil {
		return nil, models.ReadResponseCursorInfo{}, err
	}

	iter, err := storedb.Client.NewIter(&pebble.IterOptions{})
	if err != nil {
		return nil, models.ReadResponseCursorInfo{}, err
	}
	defer iter.Close()

	var startKey []byte
	if reqCursor.Cursor != "" {
		mc, err := decodeMessageCursor(reqCursor.Cursor)
		if err != nil {
			return nil, models.ReadResponseCursorInfo{}, fmt.Errorf("invalid cursor: %w", err)
		}
		if mc.ThreadID != threadID {
			return nil, models.ReadResponseCursorInfo{}, fmt.Errorf("cursor thread mismatch")
		}
		startKey = []byte(keys.GenThreadMessagesGEPrefix(threadID, mc.Sequence+1))
	} else {
		startKey = []byte(keys.GenAllThreadMessagesPrefix(threadID))
	}

	var out []string
	var lastTimestamp int64
	var lastSequence uint64
	var ts int64
	count := 0

	for iter.SeekGE(startKey); iter.Valid(); iter.Next() {
		key := iter.Key()

		threadPrefix := keys.GenAllThreadMessagesPrefix(threadID)
		if !bytes.HasPrefix(key, []byte(threadPrefix)) {
			break
		}

		parsed, err := keys.ParseMessageKey(string(key))
		if err != nil {
			continue
		}
		seq, _ := keys.ParseKeySequence(parsed.Seq)

		v := append([]byte(nil), iter.Value()...)
		v, err = encryption.DecryptMessageData(thread.KMS, v)
		if err != nil {
			logger.Error("decrypt_message_failed", "threadID", threadID, "error", err)
			return nil, models.ReadResponseCursorInfo{}, fmt.Errorf("failed to decrypt message: %w", err)
		}

		var msgData models.Message
		if err := json.Unmarshal(v, &msgData); err == nil {
			ts = msgData.TS
		}

		out = append(out, string(v))
		lastTimestamp = ts
		lastSequence = seq
		count++

		if count >= reqCursor.Limit {
			break
		}
	}

	hasMore := iter.Valid() && bytes.HasPrefix(iter.Key(), startKey[:bytes.LastIndexByte(startKey, ':')+1])

	var nextCursor string
	if len(out) > 0 && hasMore {
		nextCursor, err = encodeMessageCursor(threadID, lastTimestamp, lastSequence)
		if err != nil {
			return nil, models.ReadResponseCursorInfo{}, err
		}
	}

	respCursor := models.ReadResponseCursorInfo{
		Cursor:     nextCursor,
		HasMore:    hasMore,
		TotalCount: threadIndexes.End,
		LastSeq:    lastSequence,
	}

	return out, respCursor, iter.Error()
}

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
