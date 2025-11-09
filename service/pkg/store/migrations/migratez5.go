package migrations

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/cockroachdb/pebble"
	"progressdb/pkg/config"
	"progressdb/pkg/state/logger"
)

const (
	TSPadWidth  = 20
	SeqPadWidth = 9
)

func PadTS(ts int64) string {
	return fmt.Sprintf("%0*d", TSPadWidth, ts)
}

func PadSeq(seq uint64) string {
	return fmt.Sprintf("%0*d", SeqPadWidth, seq)
}

func GenThreadKey(threadTS int64) string {
	return fmt.Sprintf("t:%s", PadTS(threadTS))
}

func GenMessageKey(threadTS, messageTS int64, seq uint64) string {
	return fmt.Sprintf("t:%s:m:%s:%s", PadTS(threadTS), PadTS(messageTS), PadSeq(seq))
}

func GenUserOwnsThread(userID string, threadTS int64) string {
	return fmt.Sprintf("rel:u:%s:t:%s", userID, PadTS(threadTS))
}

func GenThreadHasUser(threadTS int64, userID string) string {
	return fmt.Sprintf("rel:t:%s:u:%s", PadTS(threadTS), userID)
}

type OldThread struct {
	ID        string `json:"id"`
	Title     string `json:"title,omitempty"`
	Author    string `json:"author"`
	Slug      string `json:"slug,omitempty"`
	CreatedTS int64  `json:"created_ts,omitempty"`
	UpdatedTS int64  `json:"updated_ts,omitempty"`
}

type OldMessage struct {
	ID      string      `json:"id"`
	Thread  string      `json:"thread"`
	Author  string      `json:"author"`
	Role    string      `json:"role"`
	TS      int64       `json:"ts"`
	Body    interface{} `json:"body"`
	ReplyTo string      `json:"reply_to,omitempty"`
	Deleted bool        `json:"deleted,omitempty"`
}

type ThreadData struct {
	Thread   OldThread    `json:"thread"`
	Messages []OldMessage `json:"messages"`
}

type MigrationRecords struct {
	Threads  []ThreadRecord  `json:"threads"`
	Messages []MessageRecord `json:"messages"`
	Indexes  []IndexRecord   `json:"indexes"`
}

type ThreadRecord struct {
	Key       string `json:"key"`
	Title     string `json:"title"`
	Author    string `json:"author"`
	Slug      string `json:"slug,omitempty"`
	CreatedTS int64  `json:"created_ts"`
	UpdatedTS int64  `json:"updated_ts"`
	Deleted   bool   `json:"deleted,omitempty"`
}

type MessageRecord struct {
	Key       string      `json:"key"`
	Thread    string      `json:"thread"`
	Author    string      `json:"author"`
	Role      string      `json:"role"`
	CreatedTS int64       `json:"created_ts"`
	UpdatedTS int64       `json:"updated_ts"`
	Body      interface{} `json:"body"`
	ReplyTo   string      `json:"reply_to,omitempty"`
	Deleted   bool        `json:"deleted,omitempty"`
}

type IndexRecord struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

func MigrateToRecords(ctx context.Context) (*MigrationRecords, error) {
	cfg := config.GetConfig()
	if cfg == nil {
		return nil, fmt.Errorf("service configuration not available")
	}

	oldKeyHex := cfg.Encryption.KMS.MasterKeyHex
	if oldKeyHex == "" {
		return nil, fmt.Errorf("old encryption key not configured in encryption.kms.master_key_hex")
	}

	sourcePath := cfg.Server.DBPath
	if sourcePath == "" {
		return nil, fmt.Errorf("database path not configured")
	}

	logger.Info("Starting migration to records",
		"source", sourcePath,
		"mode", "records")

	threadDataMap, err := extractAndDecryptData(sourcePath, oldKeyHex)
	if err != nil {
		return nil, fmt.Errorf("failed to extract and decrypt data: %w", err)
	}

	records := convertToRecords(threadDataMap)

	logger.Info("Migration to records completed",
		"threads", len(records.Threads),
		"messages", len(records.Messages),
		"indexes", len(records.Indexes))

	return records, nil
}

func MigrateToStore(ctx context.Context, storeDB, indexDB *pebble.DB) error {
	records, err := MigrateToRecords(ctx)
	if err != nil {
		return fmt.Errorf("failed to get migration records: %w", err)
	}

	logger.Info("Starting migration to store databases",
		"threads", len(records.Threads),
		"messages", len(records.Messages),
		"indexes", len(records.Indexes))

	if err := writeRecordsToStore(records, storeDB); err != nil {
		return fmt.Errorf("failed to write to store database: %w", err)
	}

	if err := writeRecordsToIndex(records, indexDB); err != nil {
		return fmt.Errorf("failed to write to index database: %w", err)
	}

	logger.Info("Migration to store databases completed successfully")
	return nil
}

func extractAndDecryptData(sourcePath, oldKeyHex string) (map[int64]*ThreadData, error) {
	db, err := pebble.Open(sourcePath, &pebble.Options{
		ReadOnly: true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to open source database: %w", err)
	}
	defer db.Close()

	threadDataMap := make(map[int64]*ThreadData)

	iter, _ := db.NewIter(nil)
	defer iter.Close()

	for iter.First(); iter.Valid(); iter.Next() {
		key := string(iter.Key())
		value := iter.Value()

		decryptedValue, err := decryptValue(value, oldKeyHex)
		if err != nil {
			logger.Warn("Failed to decrypt value", "key", key, "error", err)
			continue
		}

		if strings.HasPrefix(key, "thread:") && strings.HasSuffix(key, ":meta") {
			var thread OldThread
			if err := json.Unmarshal(decryptedValue, &thread); err != nil {
				logger.Warn("Failed to parse thread", "key", key, "error", err)
				continue
			}

			threadTS := extractThreadTSFromID(thread.ID)
			if _, exists := threadDataMap[threadTS]; !exists {
				threadDataMap[threadTS] = &ThreadData{
					Thread:   thread,
					Messages: []OldMessage{},
				}
			} else {
				threadDataMap[threadTS].Thread = thread
			}

		} else if strings.HasPrefix(key, "version:msg:") || (strings.HasPrefix(key, "thread:") && strings.Contains(key, ":msg:")) {
			var message OldMessage
			if err := json.Unmarshal(decryptedValue, &message); err != nil {
				logger.Warn("Failed to parse message", "key", key, "error", err)
				continue
			}

			threadTS := extractThreadTSFromID(message.Thread)
			if _, exists := threadDataMap[threadTS]; !exists {
				threadDataMap[threadTS] = &ThreadData{
					Thread:   OldThread{},
					Messages: []OldMessage{},
				}
			}

			threadDataMap[threadTS].Messages = append(threadDataMap[threadTS].Messages, message)
		}
	}

	for _, threadData := range threadDataMap {
		sort.Slice(threadData.Messages, func(i, j int) bool {
			return threadData.Messages[i].TS < threadData.Messages[j].TS
		})
	}

	return threadDataMap, nil
}

func convertToRecords(threadDataMap map[int64]*ThreadData) *MigrationRecords {
	records := &MigrationRecords{
		Threads:  []ThreadRecord{},
		Messages: []MessageRecord{},
		Indexes:  []IndexRecord{},
	}

	var threadTSs []int64
	for threadTS := range threadDataMap {
		threadTSs = append(threadTSs, threadTS)
	}
	sort.Slice(threadTSs, func(i, j int) bool {
		return threadTSs[i] < threadTSs[j]
	})

	for _, threadTS := range threadTSs {
		threadData := threadDataMap[threadTS]
		threadKey := GenThreadKey(threadTS)

		threadRecord := ThreadRecord{
			Key:       threadKey,
			Title:     threadData.Thread.Title,
			Author:    threadData.Thread.Author,
			Slug:      threadData.Thread.Slug,
			CreatedTS: threadData.Thread.CreatedTS,
			UpdatedTS: threadData.Thread.UpdatedTS,
			Deleted:   false,
		}
		records.Threads = append(records.Threads, threadRecord)

		for i, oldMessage := range threadData.Messages {
			messageRecord := MessageRecord{
				Key:       GenMessageKey(threadTS, oldMessage.TS, uint64(i)),
				Thread:    threadKey,
				Author:    oldMessage.Author,
				Role:      oldMessage.Role,
				CreatedTS: oldMessage.TS,
				UpdatedTS: oldMessage.TS,
				Body:      oldMessage.Body,
				ReplyTo:   oldMessage.ReplyTo,
				Deleted:   oldMessage.Deleted,
			}
			records.Messages = append(records.Messages, messageRecord)
		}

		if len(threadData.Messages) > 0 {
			threadTSStr := PadTS(threadTS)
			firstMsgTS := threadData.Messages[0].TS
			lastMsgTS := threadData.Messages[len(threadData.Messages)-1].TS

			records.Indexes = append(records.Indexes,
				IndexRecord{Key: fmt.Sprintf("idx:t:%s:ms:start", threadTSStr), Value: "1"},
				IndexRecord{Key: fmt.Sprintf("idx:t:%s:ms:end", threadTSStr), Value: fmt.Sprintf("%d", len(threadData.Messages))},
				IndexRecord{Key: fmt.Sprintf("idx:t:%s:ms:lc", threadTSStr), Value: fmt.Sprintf("%d", firstMsgTS)},
				IndexRecord{Key: fmt.Sprintf("idx:t:%s:ms:lu", threadTSStr), Value: fmt.Sprintf("%d", lastMsgTS)},
			)
		} else {
			threadTSStr := PadTS(threadTS)
			records.Indexes = append(records.Indexes,
				IndexRecord{Key: fmt.Sprintf("idx:t:%s:ms:start", threadTSStr), Value: "0"},
				IndexRecord{Key: fmt.Sprintf("idx:t:%s:ms:end", threadTSStr), Value: "0"},
			)
		}

		if threadData.Thread.Author != "" {
			records.Indexes = append(records.Indexes,
				IndexRecord{Key: GenUserOwnsThread(threadData.Thread.Author, threadTS), Value: "1"},
				IndexRecord{Key: GenThreadHasUser(threadTS, threadData.Thread.Author), Value: "1"},
			)
		}
	}

	return records
}

func writeRecordsToStore(records *MigrationRecords, storeDB *pebble.DB) error {
	batch := storeDB.NewBatch()
	defer batch.Close()

	for _, thread := range records.Threads {
		threadJSON, err := json.Marshal(thread)
		if err != nil {
			return fmt.Errorf("failed to marshal thread %s: %w", thread.Key, err)
		}
		if err := batch.Set([]byte(thread.Key), threadJSON, nil); err != nil {
			return fmt.Errorf("failed to store thread %s: %w", thread.Key, err)
		}
	}

	for _, message := range records.Messages {
		messageJSON, err := json.Marshal(message)
		if err != nil {
			return fmt.Errorf("failed to marshal message %s: %w", message.Key, err)
		}
		if err := batch.Set([]byte(message.Key), messageJSON, nil); err != nil {
			return fmt.Errorf("failed to store message %s: %w", message.Key, err)
		}
	}

	return batch.Commit(nil)
}

func writeRecordsToIndex(records *MigrationRecords, indexDB *pebble.DB) error {
	batch := indexDB.NewBatch()
	defer batch.Close()

	for _, index := range records.Indexes {
		if err := batch.Set([]byte(index.Key), []byte(index.Value), nil); err != nil {
			return fmt.Errorf("failed to store index %s: %w", index.Key, err)
		}
	}

	return batch.Commit(nil)
}

func decryptValue(value []byte, oldKeyHex string) ([]byte, error) {
	if likelyJSON(value) {
		return value, nil
	}

	return decryptWithOldAES(value, oldKeyHex)
}

func decryptWithOldAES(data []byte, keyHex string) ([]byte, error) {
	key, err := hex.DecodeString(keyHex)
	if err != nil {
		return nil, fmt.Errorf("failed to decode old encryption key: %w", err)
	}

	if len(key) != 32 {
		return nil, fmt.Errorf("old encryption key must be 32 bytes (AES-256)")
	}

	if len(data) < 12 {
		return nil, fmt.Errorf("ciphertext too short for old format")
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create AES cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	nonce := data[:nonceSize]
	ciphertext := data[nonceSize:]

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt: %w", err)
	}

	return plaintext, nil
}

func likelyJSON(b []byte) bool {
	for _, c := range b {
		if c == ' ' || c == '\n' || c == '\r' || c == '\t' {
			continue
		}
		return c == '{' || c == '['
	}
	return false
}

func extractThreadTSFromID(threadID string) int64 {
	parts := strings.Split(threadID, "-")
	if len(parts) >= 2 {
		if ts, err := strconv.ParseInt(parts[1], 10, 64); err == nil {
			return ts
		}
	}
	return 0
}
