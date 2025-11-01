package messages

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"

	"progressdb/pkg/models"
	"progressdb/pkg/state/logger"
	"progressdb/pkg/store/encryption"
	"progressdb/pkg/store/features/threads"

	"progressdb/pkg/state/telemetry"
	"progressdb/pkg/store/db/indexdb"
	"progressdb/pkg/store/db/storedb"
	"progressdb/pkg/store/keys"

	"github.com/cockroachdb/pebble"
)

// Move the cursor struct type up for reuse and clarity.
type messageCursor struct {
	ThreadKey string `json:"thread_key"`
	Timestamp int64  `json:"timestamp"`
	Sequence  uint64 `json:"sequence"`
}

func ListMessages(threadKey string, reqCursor models.ReadRequestCursorInfo) ([]string, models.ReadResponseCursorInfo, error) {
	tr := telemetry.Track("storedb.list_messages_cursor")
	defer tr.Finish()

	if storedb.Client == nil {
		return nil, models.ReadResponseCursorInfo{}, fmt.Errorf("pebble not opened; call storedb.Open first")
	}

	threadStr, err := threads.GetThreadData(threadKey)
	if err != nil {
		return nil, models.ReadResponseCursorInfo{}, err
	}
	var thread models.Thread
	if err := json.Unmarshal([]byte(threadStr), &thread); err != nil {
		return nil, models.ReadResponseCursorInfo{}, err
	}

	threadIndexes, err := indexdb.GetThreadMessageIndexData(threadKey)
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
		if mc.ThreadKey != threadKey {
			return nil, models.ReadResponseCursorInfo{}, fmt.Errorf("cursor thread mismatch")
		}
		prefix, err := keys.GenThreadMessagesGEPrefix(threadKey, mc.Sequence+1)
		if err != nil {
			return nil, models.ReadResponseCursorInfo{}, fmt.Errorf("failed to generate prefix: %w", err)
		}
		startKey = []byte(prefix)
	} else {
		prefix, err := keys.GenAllThreadMessagesPrefix(threadKey)
		if err != nil {
			return nil, models.ReadResponseCursorInfo{}, fmt.Errorf("failed to generate prefix: %w", err)
		}
		startKey = []byte(prefix)
	}

	var out []string
	var lastTimestamp int64
	var lastSequence uint64
	var ts int64
	count := 0

	for iter.SeekGE(startKey); iter.Valid(); iter.Next() {
		key := iter.Key()

		threadPrefix, err := keys.GenAllThreadMessagesPrefix(threadKey)
		if err != nil {
			return nil, models.ReadResponseCursorInfo{}, fmt.Errorf("failed to generate thread prefix: %w", err)
		}
		if !bytes.HasPrefix(key, []byte(threadPrefix)) {
			break
		}

		parsed, err := keys.ParseKey(string(key))
		if err != nil {
			continue
		}
		if parsed.Type != keys.KeyTypeMessage {
			continue
		}
		seq, _ := keys.ParseKeySequence(parsed.Seq)

		v := append([]byte(nil), iter.Value()...)
		v, err = encryption.DecryptMessageData(thread.KMS, v)
		if err != nil {
			logger.Error("decrypt_message_failed", "threadKey", threadKey, "error", err)
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
		nextCursor, err = encodeMessageCursor(threadKey, lastTimestamp, lastSequence)
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

func encodeMessageCursor(threadKey string, timestamp int64, sequence uint64) (string, error) {
	cursor := messageCursor{
		ThreadKey: threadKey,
		Timestamp: timestamp,
		Sequence:  sequence,
	}
	data, err := json.Marshal(cursor)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(data), nil
}

func decodeMessageCursor(cursor string) (messageCursor, error) {
	var result messageCursor

	data, err := base64.StdEncoding.DecodeString(cursor)
	if err != nil {
		return result, err
	}
	err = json.Unmarshal(data, &result)
	return result, err
}
