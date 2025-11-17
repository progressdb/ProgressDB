package migrations

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"progressdb/pkg/config"
	"progressdb/pkg/models"
	"progressdb/pkg/state/logger"
	"progressdb/pkg/store/encryption"

	"github.com/cockroachdb/pebble"
)

const (
	SeqPadWidth = 9 // e.g. %09d
)

func PadSeq(seq uint64) string {
	return fmt.Sprintf("%0*d", SeqPadWidth, seq)
}

func GenThreadKey(threadTS int64) string {
	return fmt.Sprintf("t:%d", threadTS)
}

func GenMessageKey(threadTS, messageTS int64, seq uint64) string {
	return fmt.Sprintf("t:%d:m:%d:%s", threadTS, messageTS, PadSeq(seq))
}

func GenUserOwnsThread(userID string, threadTS int64) string {
	return fmt.Sprintf("rel:u:%s:t:%d", userID, threadTS)
}

func GenThreadHasUser(threadTS int64, userID string) string {
	return fmt.Sprintf("rel:t:%d:u:%s", threadTS, userID)
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
	Seq     uint64      `json:"seq,omitempty"` // Preserve original sequence
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
	CreatedTS int64  `json:"created_ts"`
	UpdatedTS int64  `json:"updated_ts"`
	Deleted   bool   `json:"deleted,omitempty"`
}

type MessageRecord struct {
	Key       string      `json:"key"`
	Thread    string      `json:"thread"`
	Author    string      `json:"author"`
	CreatedTS int64       `json:"created_ts"`
	UpdatedTS int64       `json:"updated_ts"`
	Body      interface{} `json:"body"`
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

	logger.Info("Starting key-first migration to records",
		"source", sourcePath,
		"mode", "records")

	// Step 1: Generate clean key mappings first
	if err := DumpAndConvertKeys(sourcePath); err != nil {
		return nil, fmt.Errorf("failed to generate key mappings: %w", err)
	}

	// Load the generated key mappings
	keyMappings, err := loadKeyMappings("key_mapping_v2.csv")
	if err != nil {
		return nil, fmt.Errorf("failed to load key mappings: %w", err)
	}

	// Step 2: Extract and decrypt data using clean mappings
	threadDataMap, err := extractAndDecryptDataWithMappings(sourcePath, oldKeyHex, keyMappings)
	if err != nil {
		return nil, fmt.Errorf("failed to extract and decrypt data: %w", err)
	}

	// Step 3: Convert to records using clean keys
	records := convertToRecordsWithMappings(threadDataMap, keyMappings)

	logger.Info("Key-first migration to records completed",
		"threads", len(records.Threads),
		"messages", len(records.Messages),
		"indexes", len(records.Indexes),
		"keyMappings", len(keyMappings))

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

func writeRecordsToStore(records *MigrationRecords, storeDB *pebble.DB) error {
	batch := storeDB.NewBatch()
	defer batch.Close()

	// Process threads first to establish KMS if needed
	threadKMSMap := make(map[string]*models.KMSMeta)
	for _, thread := range records.Threads {
		// Create Thread model
		threadModel := &models.Thread{
			Key:       thread.Key,
			Title:     thread.Title,
			Author:    thread.Author,
			CreatedTS: thread.CreatedTS,
			UpdatedTS: thread.UpdatedTS,
			Deleted:   thread.Deleted,
		}

		// Provision KMS for thread if encryption is enabled
		if encryption.EncryptionEnabled() {
			kmsMeta, err := encryption.ProvisionThreadKMS(thread.Key)
			if err != nil {
				return fmt.Errorf("failed to provision KMS for thread %s: %w", thread.Key, err)
			}
			if kmsMeta != nil {
				threadModel.WithKMS(*kmsMeta)
				threadKMSMap[thread.Key] = kmsMeta
			}
		}

		threadJSON, err := json.Marshal(threadModel)
		if err != nil {
			return fmt.Errorf("failed to marshal thread %s: %w", thread.Key, err)
		}
		if err := batch.Set([]byte(thread.Key), threadJSON, nil); err != nil {
			return fmt.Errorf("failed to store thread %s: %w", thread.Key, err)
		}
	}

	// Process messages with thread KMS if available
	for _, message := range records.Messages {
		// Create Message model
		messageModel := &models.Message{
			Key:       message.Key,
			Thread:    message.Thread,
			Author:    message.Author,
			CreatedTS: message.CreatedTS,
			UpdatedTS: message.UpdatedTS,
			Body:      message.Body,
			Deleted:   message.Deleted,
		}

		// Encrypt message data if encryption is enabled
		messageJSON, err := json.Marshal(messageModel)
		if err != nil {
			return fmt.Errorf("failed to marshal message %s: %w", message.Key, err)
		}

		if encryption.EncryptionEnabled() {
			threadKMS := threadKMSMap[message.Thread]
			if threadKMS == nil {
				return fmt.Errorf("no KMS metadata found for thread %s when encrypting message %s", message.Thread, message.Key)
			}

			if encryption.EncryptionHasFieldPolicy() {
				// Use field-level encryption
				encBody, err := encryption.EncryptMessageBody(messageModel, models.Thread{KMS: threadKMS})
				if err != nil {
					return fmt.Errorf("failed to encrypt message body %s: %w", message.Key, err)
				}
				messageModel.Body = encBody
				messageJSON, err = json.Marshal(messageModel)
				if err != nil {
					return fmt.Errorf("failed to marshal encrypted message %s: %w", message.Key, err)
				}
			} else {
				// Use full message encryption
				enc, _, err := encryption.EncryptWithDEK(threadKMS.KeyID, messageJSON, nil)
				if err != nil {
					return fmt.Errorf("failed to encrypt message %s: %w", message.Key, err)
				}
				messageJSON = enc
			}
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

// loadKeyMappings loads key mappings from CSV file
func loadKeyMappings(filename string) (map[string]*KeyMapping, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read key mappings file: %w", err)
	}

	lines := strings.Split(string(data), "\n")
	mappings := make(map[string]*KeyMapping)

	// Skip header
	for i, line := range lines {
		if i == 0 || line == "" {
			continue
		}

		parts := strings.Split(line, ",")
		if len(parts) >= 2 {
			mappings[parts[0]] = &KeyMapping{
				OldKey: parts[0],
				NewKey: parts[1],
			}
		}
	}

	logger.Info("Loaded key mappings", "count", len(mappings))
	return mappings, nil
}

// extractAndDecryptDataWithMappings extracts data using clean key mappings
func extractAndDecryptDataWithMappings(sourcePath, oldKeyHex string, keyMappings map[string]*KeyMapping) (map[int64]*ThreadData, error) {
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

		} else if strings.HasPrefix(key, "thread:") && strings.Contains(key, ":msg:") {
			var message OldMessage
			if err := json.Unmarshal(decryptedValue, &message); err != nil {
				logger.Warn("Failed to parse message", "key", key, "error", err)
				continue
			}

			// Use clean key mapping if available
			if mapping, exists := keyMappings[key]; exists {
				// Extract timestamp and sequence from new key
				if parsed, err := ParseMessageKey(mapping.NewKey); err == nil {
					message.TS = parsed.Timestamp
					message.Seq = uint64(parsed.NewSeq)
				}
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

	// Sort messages within each thread using clean timestamps and sequences
	for _, threadData := range threadDataMap {
		sort.Slice(threadData.Messages, func(i, j int) bool {
			if threadData.Messages[i].TS != threadData.Messages[j].TS {
				return threadData.Messages[i].TS < threadData.Messages[j].TS
			}
			return threadData.Messages[i].Seq < threadData.Messages[j].Seq
		})
	}

	return threadDataMap, nil
}

// convertToRecordsWithMappings converts to records using clean key mappings
func convertToRecordsWithMappings(threadDataMap map[int64]*ThreadData, keyMappings map[string]*KeyMapping) *MigrationRecords {
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
			CreatedTS: threadData.Thread.CreatedTS,
			UpdatedTS: threadData.Thread.UpdatedTS,
			Deleted:   false,
		}
		records.Threads = append(records.Threads, threadRecord)

		// Generate message records using clean sequences
		for i, oldMessage := range threadData.Messages {
			// Use the correct new key format: t:<thread_ts>:m:<message_ts>:<seq>
			newKey := fmt.Sprintf("t:%d:m:%d:%s",
				threadTS,          // thread timestamp
				oldMessage.TS,     // message timestamp
				PadSeq(uint64(i))) // sequence

			messageRecord := MessageRecord{
				Key:       newKey,
				Thread:    threadKey,
				Author:    oldMessage.Author,
				CreatedTS: oldMessage.TS,
				UpdatedTS: oldMessage.TS,
				Body:      oldMessage.Body,
				Deleted:   oldMessage.Deleted,
			}
			records.Messages = append(records.Messages, messageRecord)
		}

		// Generate indexes using clean keys
		if len(threadData.Messages) > 0 {
			threadTSStr := fmt.Sprintf("%d", threadTS)
			firstMsgTS := threadData.Messages[0].TS
			lastMsgTS := threadData.Messages[len(threadData.Messages)-1].TS

			records.Indexes = append(records.Indexes,
				IndexRecord{Key: fmt.Sprintf("idx:t:%s:ms:start", threadTSStr), Value: "0"},
				IndexRecord{Key: fmt.Sprintf("idx:t:%s:ms:end", threadTSStr), Value: fmt.Sprintf("%d", len(threadData.Messages))},
				IndexRecord{Key: fmt.Sprintf("idx:t:%s:ms:lc", threadTSStr), Value: fmt.Sprintf("%d", firstMsgTS)},
				IndexRecord{Key: fmt.Sprintf("idx:t:%s:ms:lu", threadTSStr), Value: fmt.Sprintf("%d", lastMsgTS)},
			)
		} else {
			threadTSStr := fmt.Sprintf("%d", threadTS)
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
