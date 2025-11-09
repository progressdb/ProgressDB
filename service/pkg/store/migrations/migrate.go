package migrations

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/cockroachdb/pebble"
	"progressdb/pkg/config"
	"progressdb/pkg/state/logger"
)

// Constants matching service
const (
	TSPadWidth  = 20 // e.g. %020d
	SeqPadWidth = 9  // e.g. %09d
)

// Key generation helpers
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

// Old models from 0.1.2
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

// New models for 0.5.0
type NewThread struct {
	Key       string `json:"key"`
	Title     string `json:"title"`
	Author    string `json:"author"`
	Slug      string `json:"slug,omitempty"`
	CreatedTS int64  `json:"created_ts"`
	UpdatedTS int64  `json:"updated_ts"`
	Deleted   bool   `json:"deleted,omitempty"`
}

type NewMessage struct {
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

type SystemInfo struct {
	Version    string `json:"version"`
	MigratedAt string `json:"migrated_at"`
	Threads    int    `json:"threads_count"`
	Messages   int    `json:"messages_count"`
}

type ThreadData struct {
	Thread   OldThread    `json:"thread"`
	Messages []OldMessage `json:"messages"`
}

type MigrationStats struct {
	ThreadsMigrated      int
	MessagesMigrated     int
	IndexesCreated       int
	RelationshipsCreated int
}

// MigrationRecords holds the structured data from migration
type MigrationRecords struct {
	Threads  []ThreadRecord  `json:"threads"`
	Messages []MessageRecord `json:"messages"`
	Indexes  []IndexRecord   `json:"indexes"`
}

// ThreadRecord represents a migrated thread
type ThreadRecord struct {
	Key       string `json:"key"`
	Title     string `json:"title"`
	Author    string `json:"author"`
	Slug      string `json:"slug,omitempty"`
	CreatedTS int64  `json:"created_ts"`
	UpdatedTS int64  `json:"updated_ts"`
	Deleted   bool   `json:"deleted,omitempty"`
}

// MessageRecord represents a migrated message
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

// IndexRecord represents a migrated index entry
type IndexRecord struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// MigrateToRecords extracts and returns migration data from old 0.1.2 database
func MigrateToRecords(ctx context.Context) (*MigrationRecords, error) {
	cfg := config.GetConfig()
	if cfg == nil {
		return nil, fmt.Errorf("service configuration not available")
	}

	// Get old encryption key from current config
	oldKeyHex := cfg.Encryption.KMS.MasterKeyHex
	if oldKeyHex == "" {
		return nil, fmt.Errorf("old encryption key not configured in encryption.kms.master_key_hex")
	}

	// Get source database path
	sourcePath := cfg.Server.DBPath
	if sourcePath == "" {
		return nil, fmt.Errorf("database path not configured")
	}

	logger.Info("Starting migration to records",
		"source", sourcePath,
		"mode", "records")

	// Extract data from old database
	threadDataMap, err := extractAndDecryptData(sourcePath, oldKeyHex)
	if err != nil {
		return nil, fmt.Errorf("failed to extract and decrypt data: %w", err)
	}

	// Convert to records format
	records := convertToRecords(threadDataMap)

	logger.Info("Migration to records completed",
		"threads", len(records.Threads),
		"messages", len(records.Messages),
		"indexes", len(records.Indexes))

	return records, nil
}

// MigrateToStore migrates data directly to existing store and index databases
func MigrateToStore(ctx context.Context, storeDB, indexDB *pebble.DB) error {
	// Get records first
	records, err := MigrateToRecords(ctx)
	if err != nil {
		return fmt.Errorf("failed to get migration records: %w", err)
	}

	logger.Info("Starting migration to store databases",
		"threads", len(records.Threads),
		"messages", len(records.Messages),
		"indexes", len(records.Indexes))

	// Write to store database (threads and messages)
	if err := writeRecordsToStore(records, storeDB); err != nil {
		return fmt.Errorf("failed to write to store database: %w", err)
	}

	// Write to index database
	if err := writeRecordsToIndex(records, indexDB); err != nil {
		return fmt.Errorf("failed to write to index database: %w", err)
	}

	logger.Info("Migration to store databases completed successfully")
	return nil
}

// extractAndDecryptData extracts data from old 0.1.2 database
func extractAndDecryptData(sourcePath, oldKeyHex string) (map[int64]*ThreadData, error) {
	// Open the old pebble database
	db, err := pebble.Open(sourcePath, &pebble.Options{
		ReadOnly: true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to open source database: %w", err)
	}
	defer db.Close()

	threadDataMap := make(map[int64]*ThreadData)

	// Iterate through all keys and extract data
	iter, _ := db.NewIter(nil)
	defer iter.Close()

	for iter.First(); iter.Valid(); iter.Next() {
		key := string(iter.Key())
		value := iter.Value()

		// Decrypt the value using old encryption method
		decryptedValue, err := decryptValue(value, oldKeyHex)
		if err != nil {
			logger.Warn("Failed to decrypt value", "key", key, "error", err)
			continue
		}

		// Process based on key pattern
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

	// Sort messages within each thread by timestamp
	for _, threadData := range threadDataMap {
		sort.Slice(threadData.Messages, func(i, j int) bool {
			return threadData.Messages[i].TS < threadData.Messages[j].TS
		})
	}

	return threadDataMap, nil
}

// convertToRecords converts ThreadData to MigrationRecords
func convertToRecords(threadDataMap map[int64]*ThreadData) *MigrationRecords {
	records := &MigrationRecords{
		Threads:  []ThreadRecord{},
		Messages: []MessageRecord{},
		Indexes:  []IndexRecord{},
	}

	// Sort threads by timestamp for consistent ordering
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

		// Convert thread
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

		// Convert messages
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

		// Create indexes
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

		// User relationship indexes
		if threadData.Thread.Author != "" {
			records.Indexes = append(records.Indexes,
				IndexRecord{Key: GenUserOwnsThread(threadData.Thread.Author, threadTS), Value: "1"},
				IndexRecord{Key: GenThreadHasUser(threadTS, threadData.Thread.Author), Value: "1"},
			)
		}
	}

	return records
}

// writeRecordsToStore writes threads and messages to store database
func writeRecordsToStore(records *MigrationRecords, storeDB *pebble.DB) error {
	batch := storeDB.NewBatch()
	defer batch.Close()

	// Write threads
	for _, thread := range records.Threads {
		threadJSON, err := json.Marshal(thread)
		if err != nil {
			return fmt.Errorf("failed to marshal thread %s: %w", thread.Key, err)
		}
		if err := batch.Set([]byte(thread.Key), threadJSON, nil); err != nil {
			return fmt.Errorf("failed to store thread %s: %w", thread.Key, err)
		}
	}

	// Write messages
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

// writeRecordsToIndex writes indexes to index database
func writeRecordsToIndex(records *MigrationRecords, indexDB *pebble.DB) error {
	batch := indexDB.NewBatch()
	defer batch.Close()

	// Write indexes
	for _, index := range records.Indexes {
		if err := batch.Set([]byte(index.Key), []byte(index.Value), nil); err != nil {
			return fmt.Errorf("failed to store index %s: %w", index.Key, err)
		}
	}

	return batch.Commit(nil)
}

// decryptValue decrypts data using old 0.1.2 encryption method
func decryptValue(value []byte, oldKeyHex string) ([]byte, error) {
	// Check if data looks like JSON (unencrypted)
	if likelyJSON(value) {
		return value, nil
	}

	// Try to decrypt using old AES-GCM method
	return decryptWithOldAES(value, oldKeyHex)
}

// decryptWithOldAES attempts to decrypt using the old 0.1.2 AES-GCM method
func decryptWithOldAES(data []byte, keyHex string) ([]byte, error) {
	// Decode the old encryption key from hex
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

	// Create AES cipher
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create AES cipher: %w", err)
	}

	// Create GCM
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	// Extract nonce and ciphertext
	nonce := data[:nonceSize]
	ciphertext := data[nonceSize:]

	// Decrypt
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt: %w", err)
	}

	return plaintext, nil
}

// likelyJSON checks if data looks like JSON
func likelyJSON(b []byte) bool {
	for _, c := range b {
		if c == ' ' || c == '\n' || c == '\r' || c == '\t' {
			continue
		}
		return c == '{' || c == '['
	}
	return false
}

// MigrateToJSON migrates old 0.1.2 data to new 0.5.0 JSON format (kept for CLI compatibility)
func MigrateToJSON(sourceDir, outputDir string) error {
	logger.Info("Starting migration from 0.1.2 to 0.5.0 (JSON output)",
		"source", sourceDir,
		"output", outputDir)

	// Create output directories
	storeDir := filepath.Join(outputDir, "storedb")
	indexDir := filepath.Join(outputDir, "indexdb")

	if err := os.MkdirAll(filepath.Join(storeDir, "threads"), 0755); err != nil {
		return fmt.Errorf("failed to create threads directory: %w", err)
	}

	if err := os.MkdirAll(filepath.Join(storeDir, "messages"), 0755); err != nil {
		return fmt.Errorf("failed to create messages directory: %w", err)
	}

	if err := os.MkdirAll(filepath.Join(indexDir, "indexes"), 0755); err != nil {
		return fmt.Errorf("failed to create indexes directory: %w", err)
	}

	stats := &MigrationStats{}

	// Phase 1: Load all threads and messages into memory
	logger.Info("Phase 1: Loading threads and messages into memory")
	threadDataMap, err := loadThreadData(sourceDir)
	if err != nil {
		return fmt.Errorf("failed to load thread data: %w", err)
	}

	logger.Info("Loaded threads with messages", "count", len(threadDataMap))

	// Phase 2: Migrate threads
	logger.Info("Phase 2: Migrating threads to storedb/threads")
	if err := migrateThreads(threadDataMap, storeDir, stats); err != nil {
		return fmt.Errorf("thread migration failed: %w", err)
	}

	// Phase 3: Migrate messages
	logger.Info("Phase 3: Migrating messages to storedb/messages")
	if err := migrateMessages(threadDataMap, storeDir, stats); err != nil {
		return fmt.Errorf("message migration failed: %w", err)
	}

	// Phase 4: Create indexes
	logger.Info("Phase 4: Creating indexes in indexdb/indexes")
	if err := createIndexes(threadDataMap, filepath.Join(indexDir, "indexes"), stats); err != nil {
		return fmt.Errorf("index creation failed: %w", err)
	}

	// Phase 5: Create system info
	if err := createSystemInfo(outputDir, stats); err != nil {
		return fmt.Errorf("system info creation failed: %w", err)
	}

	logger.Info("Migration completed successfully",
		"threads", stats.ThreadsMigrated,
		"messages", stats.MessagesMigrated,
		"indexes", stats.IndexesCreated,
		"relationships", stats.RelationshipsCreated)

	return nil
}

func loadThreadData(sourceDir string) (map[int64]*ThreadData, error) {
	threadDataMap := make(map[int64]*ThreadData)

	// Load threads
	threadsDir := filepath.Join(sourceDir, "threads")
	threadFiles, err := os.ReadDir(threadsDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read threads directory: %w", err)
	}

	for _, file := range threadFiles {
		if !strings.HasSuffix(file.Name(), ".json") {
			continue
		}

		filePath := filepath.Join(threadsDir, file.Name())
		data, err := os.ReadFile(filePath)
		if err != nil {
			logger.Error("Failed to read thread file", "file", file.Name(), "error", err)
			continue
		}

		var oldThread OldThread
		if err := json.Unmarshal(data, &oldThread); err != nil {
			logger.Error("Failed to unmarshal thread", "file", file.Name(), "error", err)
			continue
		}

		threadTS := extractThreadTSFromID(oldThread.ID)
		threadDataMap[threadTS] = &ThreadData{
			Thread:   oldThread,
			Messages: []OldMessage{},
		}
	}

	// Load messages and group by thread
	messagesDir := filepath.Join(sourceDir, "messages")
	messageFiles, err := os.ReadDir(messagesDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read messages directory: %w", err)
	}

	for _, file := range messageFiles {
		if !strings.HasSuffix(file.Name(), ".json") {
			continue
		}

		filePath := filepath.Join(messagesDir, file.Name())
		data, err := os.ReadFile(filePath)
		if err != nil {
			logger.Error("Failed to read message file", "file", file.Name(), "error", err)
			continue
		}

		var oldMessage OldMessage
		if err := json.Unmarshal(data, &oldMessage); err != nil {
			logger.Error("Failed to unmarshal message", "file", file.Name(), "error", err)
			continue
		}

		threadTS := extractThreadTSFromID(oldMessage.Thread)
		if threadData, exists := threadDataMap[threadTS]; exists {
			threadData.Messages = append(threadData.Messages, oldMessage)
		} else {
			logger.Warn("Message belongs to unknown thread",
				"message", oldMessage.ID,
				"thread", oldMessage.Thread)
		}
	}

	// Sort messages within each thread by timestamp
	for threadTS, threadData := range threadDataMap {
		sort.Slice(threadData.Messages, func(i, j int) bool {
			return threadData.Messages[i].TS < threadData.Messages[j].TS
		})
		logger.Debug("Thread loaded",
			"id", threadData.Thread.ID,
			"ts", threadTS,
			"messages", len(threadData.Messages))
	}

	return threadDataMap, nil
}

func migrateThreads(threadDataMap map[int64]*ThreadData, outputDir string, stats *MigrationStats) error {
	threadsDir := filepath.Join(outputDir, "threads")

	for threadTS, threadData := range threadDataMap {
		// Convert to new model
		threadKey := GenThreadKey(threadTS)
		newThread := NewThread{
			Key:       threadKey,
			Title:     threadData.Thread.Title,
			Author:    threadData.Thread.Author,
			Slug:      threadData.Thread.Slug,
			CreatedTS: threadData.Thread.CreatedTS,
			UpdatedTS: threadData.Thread.UpdatedTS,
			Deleted:   false, // Assume not deleted for initial migration
		}

		// Save as individual file
		outputFile := filepath.Join(threadsDir, fmt.Sprintf("%s.json", sanitizeKey(threadKey)))
		threadJSON, err := json.MarshalIndent(newThread, "", "  ")
		if err != nil {
			logger.Error("Failed to marshal thread", "id", threadData.Thread.ID, "error", err)
			continue
		}

		if err := os.WriteFile(outputFile, threadJSON, 0644); err != nil {
			logger.Error("Failed to save thread", "id", threadData.Thread.ID, "error", err)
			continue
		}

		stats.ThreadsMigrated++
	}

	return nil
}

func migrateMessages(threadDataMap map[int64]*ThreadData, outputDir string, stats *MigrationStats) error {
	messagesDir := filepath.Join(outputDir, "messages")

	for threadTS, threadData := range threadDataMap {
		threadKey := GenThreadKey(threadTS)

		// Migrate each message with proper sequence starting from 0
		for i, oldMessage := range threadData.Messages {
			// Convert to new model
			messageKey := GenMessageKey(threadTS, oldMessage.TS, uint64(i))
			newMessage := NewMessage{
				Key:       messageKey,
				Thread:    threadKey,
				Author:    oldMessage.Author,
				Role:      oldMessage.Role,
				CreatedTS: oldMessage.TS,
				UpdatedTS: oldMessage.TS, // Same as created for initial migration
				Body:      oldMessage.Body,
				ReplyTo:   oldMessage.ReplyTo,
				Deleted:   oldMessage.Deleted,
			}

			// Save as individual file
			outputFile := filepath.Join(messagesDir, fmt.Sprintf("%s.json", sanitizeKey(messageKey)))
			messageJSON, err := json.MarshalIndent(newMessage, "", "  ")
			if err != nil {
				logger.Error("Failed to marshal message", "id", oldMessage.ID, "error", err)
				continue
			}

			if err := os.WriteFile(outputFile, messageJSON, 0644); err != nil {
				logger.Error("Failed to save message", "id", oldMessage.ID, "error", err)
				continue
			}

			stats.MessagesMigrated++
		}
	}

	return nil
}

func createIndexes(threadDataMap map[int64]*ThreadData, outputDir string, stats *MigrationStats) error {
	for threadTS, threadData := range threadDataMap {
		threadKey := GenThreadKey(threadTS)
		messages := threadData.Messages

		// Create thread message indexes
		indexes := make(map[string]interface{})

		if len(messages) > 0 {
			// First message sequence = 0, Last message sequence = len(messages)-1
			firstSeq := PadSeq(0)
			lastSeq := PadSeq(uint64(len(messages) - 1))
			firstMsgTS := messages[0].TS
			lastMsgTS := messages[len(messages)-1].TS

			threadTSStr := PadTS(threadTS)
			indexes[fmt.Sprintf("idx:t:%s:ms:start", threadTSStr)] = firstSeq
			indexes[fmt.Sprintf("idx:t:%s:ms:end", threadTSStr)] = lastSeq
			indexes[fmt.Sprintf("idx:t:%s:ms:lc", threadTSStr)] = fmt.Sprintf("%d", firstMsgTS) // timestamp
			indexes[fmt.Sprintf("idx:t:%s:ms:lu", threadTSStr)] = fmt.Sprintf("%d", lastMsgTS)  // timestamp
		} else {
			// Empty thread - set start and end to "0"
			threadTSStr := PadTS(threadTS)
			indexes[fmt.Sprintf("idx:t:%s:ms:start", threadTSStr)] = "0"
			indexes[fmt.Sprintf("idx:t:%s:ms:end", threadTSStr)] = "0"
		}

		// User relationship indexes
		if threadData.Thread.Author != "" {
			indexes[GenUserOwnsThread(threadData.Thread.Author, threadTS)] = "1"
			indexes[GenThreadHasUser(threadTS, threadData.Thread.Author)] = "1"
			stats.RelationshipsCreated += 2
		}

		// Save indexes
		indexFile := filepath.Join(outputDir, fmt.Sprintf("%s.json", sanitizeKey(threadKey)))
		indexJSON, err := json.MarshalIndent(indexes, "", "  ")
		if err != nil {
			continue
		}

		if err := os.WriteFile(indexFile, indexJSON, 0644); err != nil {
			continue
		}

		stats.IndexesCreated += len(indexes)
	}

	return nil
}

func createSystemInfo(outputDir string, stats *MigrationStats) error {
	systemInfo := SystemInfo{
		Version:    "0.5.0",
		MigratedAt: fmt.Sprintf("%d", 1757529777000), // Current timestamp
		Threads:    stats.ThreadsMigrated,
		Messages:   stats.MessagesMigrated,
	}

	data, err := json.MarshalIndent(systemInfo, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(outputDir, "system.json"), data, 0644)
}

func extractThreadTSFromID(threadID string) int64 {
	// Extract timestamp from thread ID like "thread-1757409700041271267-1"
	parts := strings.Split(threadID, "-")
	if len(parts) >= 2 {
		if ts, err := strconv.ParseInt(parts[1], 10, 64); err == nil {
			return ts
		}
	}
	return 0
}

func sanitizeKey(key string) string {
	// Replace problematic characters with underscores
	replacer := strings.NewReplacer(
		":", "_",
		"/", "_",
		"\\", "_",
		"*", "_",
		"?", "_",
		"\"", "_",
		"<", "_",
		">", "_",
		"|", "_",
	)
	return replacer.Replace(key)
}
