package migrate

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/cockroachdb/pebble"
	"progressdb/clients/cli/config"
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

// MigrateToJSON performs the migration from 0.1.2 pebble database to 0.5.0 JSON format
func MigrateToJSON(cfg *config.Config, verbose bool) error {
	// Create timestamped output directory
	timestamp := time.Now().Format("2006-01-02_15-04-05")
	timestampedDir := filepath.Join(cfg.ToDatabase, timestamp)

	fmt.Printf("🚀 Starting migration from 0.1.2 to 0.5.0 (JSON output)\n")
	fmt.Printf("📁 Source: %s\n", cfg.FromDatabase)
	fmt.Printf("📁 Output: %s\n", timestampedDir)

	// Create output directories
	storeDir := filepath.Join(timestampedDir, "storedb")
	indexDir := filepath.Join(timestampedDir, "indexdb")
	originalDbDir := filepath.Join(timestampedDir, "originaldb")

	if err := os.MkdirAll(filepath.Join(storeDir, "threads"), 0755); err != nil {
		return fmt.Errorf("failed to create threads directory: %w", err)
	}

	if err := os.MkdirAll(filepath.Join(storeDir, "messages"), 0755); err != nil {
		return fmt.Errorf("failed to create messages directory: %w", err)
	}

	if err := os.MkdirAll(filepath.Join(indexDir, "indexes"), 0755); err != nil {
		return fmt.Errorf("failed to create indexes directory: %w", err)
	}

	// Copy original database files to originaldb directory
	fmt.Printf("📋 Copying original database files...\n")
	if err := CopyPebbleFiles(cfg, originalDbDir, verbose); err != nil {
		return fmt.Errorf("failed to copy original database files: %w", err)
	}

	stats := &MigrationStats{}

	// Phase 1: Decrypt and extract data from pebble database
	fmt.Printf("📋 Phase 1: Decrypting and extracting data from pebble database...\n")
	threadDataMap, err := extractAndDecryptData(cfg, verbose)
	if err != nil {
		return fmt.Errorf("failed to extract and decrypt data: %w", err)
	}

	fmt.Printf("  Loaded %d threads with their messages\n", len(threadDataMap))

	// Phase 2: Migrate threads
	fmt.Printf("📋 Phase 2: Migrating threads to storedb/threads...\n")
	if err := migrateThreads(threadDataMap, storeDir, stats); err != nil {
		return fmt.Errorf("thread migration failed: %w", err)
	}

	// Phase 3: Migrate messages
	fmt.Printf("💬 Phase 3: Migrating messages to storedb/messages...\n")
	if err := migrateMessages(threadDataMap, storeDir, stats); err != nil {
		return fmt.Errorf("message migration failed: %w", err)
	}

	// Phase 4: Create indexes
	fmt.Printf("🗂️ Phase 4: Creating indexes in indexdb/indexes...\n")
	if err := createIndexes(threadDataMap, filepath.Join(indexDir, "indexes"), stats); err != nil {
		return fmt.Errorf("index creation failed: %w", err)
	}

	// Phase 5: Create system info
	if err := createSystemInfo(timestampedDir, stats); err != nil {
		return fmt.Errorf("system info creation failed: %w", err)
	}

	// Print final statistics
	fmt.Printf("\n🎉 Migration completed successfully!\n")
	fmt.Printf("📊 Statistics:\n")
	fmt.Printf("  Threads migrated: %d\n", stats.ThreadsMigrated)
	fmt.Printf("  Messages migrated: %d\n", stats.MessagesMigrated)
	fmt.Printf("  Indexes created: %d\n", stats.IndexesCreated)
	fmt.Printf("  Relationships created: %d\n", stats.RelationshipsCreated)
	fmt.Printf("\n📁 Output structure:\n")
	fmt.Printf("📂 %s/\n", cfg.ToDatabase)
	fmt.Printf("  📂 storedb/\n")
	fmt.Printf("    📂 threads/     - Thread metadata\n")
	fmt.Printf("    📂 messages/    - Message content\n")
	fmt.Printf("  📂 indexdb/\n")
	fmt.Printf("    📂 indexes/     - Performance indexes\n")
	fmt.Printf("  📄 system.json   - System metadata\n")

	return nil
}

func extractAndDecryptData(cfg *config.Config, verbose bool) (map[int64]*ThreadData, error) {
	// Create timestamped directory path
	timestamp := time.Now().Format("2006-01-02_15-04-05")
	timestampedDir := filepath.Join(cfg.ToDatabase, timestamp)
	originalDbDir := filepath.Join(timestampedDir, "originaldb")

	// Open the pebble database from originaldb folder
	db, err := pebble.Open(originalDbDir, &pebble.Options{
		ReadOnly: true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to open pebble database: %w", err)
	}
	defer db.Close()

	threadDataMap := make(map[int64]*ThreadData)

	// Iterate through all keys and extract data
	iter, _ := db.NewIter(nil)
	defer iter.Close()

	threadCount := 0
	messageCount := 0
	totalKeys := 0
	versionMsgKeys := 0

	for iter.First(); iter.Valid(); iter.Next() {
		totalKeys++
		key := string(iter.Key())
		value := iter.Value()

		if strings.HasPrefix(key, "version:msg:") {
			versionMsgKeys++
		}

		// Decrypt the value using old encryption method
		decryptedValue, err := decryptValue(value, cfg.OldEncryptionKey)
		if err != nil {
			if verbose {
				fmt.Printf("⚠️  Failed to decrypt value for key %s: %v\n", key, err)
			}
			continue
		}

		// Debug: check if version:msg: keys are being decrypted properly
		if verbose && strings.HasPrefix(key, "version:msg:") && len(decryptedValue) > 0 {
			if decryptedValue[0] != '{' && decryptedValue[0] != '[' {
				fmt.Printf("🔍 Version message %s still encrypted after decryption (first byte: %d)\n", key, decryptedValue[0])
			} else {
				fmt.Printf("✅ Version message %s successfully decrypted\n", key)
			}
		}

		// Parse and save based on key type
		isThreadKey := strings.HasPrefix(key, "thread:") && strings.HasSuffix(key, ":meta")
		// Regular message keys (thread:*:msg:*)
		isMessageKey := strings.Contains(key, ":msg:") && !strings.HasPrefix(key, "version:msg:")

		if verbose && (strings.Contains(key, "01757619432601731301") || strings.Contains(key, "01757963423216750249")) {
			fmt.Printf("🔍 Key: %s, isThreadKey: %v, isMessageKey: %v\n", key, isThreadKey, isMessageKey)
		}

		if isThreadKey {
			if verbose && (strings.Contains(key, "01757619432601731301") || strings.Contains(key, "01757963423216750249")) {
				fmt.Printf("🔍 Processing as thread: %s\n", key)
			}
			var oldThread OldThread
			if err := json.Unmarshal(decryptedValue, &oldThread); err != nil {
				if verbose {
					fmt.Printf("⚠️  Failed to unmarshal thread %s: %v\n", key, err)
				}
				continue
			}

			threadTS := extractThreadTSFromID(oldThread.ID)
			threadDataMap[threadTS] = &ThreadData{
				Thread:   oldThread,
				Messages: []OldMessage{},
			}
			threadCount++
		} else if isMessageKey {
			if verbose && (strings.Contains(key, "01757619432601731301") || strings.Contains(key, "01757963423216750249")) {
				fmt.Printf("🔍 Processing as message: %s\n", key)
			}
			var oldMessage OldMessage
			if err := json.Unmarshal(decryptedValue, &oldMessage); err != nil {
				if verbose {
					fmt.Printf("⚠️  Failed to unmarshal message %s: %v\n", key, err)
					fmt.Printf("   Raw data (first 100 chars): %q\n", string(decryptedValue[:min(100, len(decryptedValue))]))
				}
				continue
			}

			threadTS := extractThreadTSFromID(oldMessage.Thread)
			if threadData, exists := threadDataMap[threadTS]; exists {
				threadData.Messages = append(threadData.Messages, oldMessage)
				if verbose && messageCount < 5 {
					fmt.Printf("✅ Added message %s to thread %d\n", oldMessage.ID, threadTS)
				}
			} else {
				if verbose {
					fmt.Printf("⚠️  Message %s belongs to unknown thread %s (TS: %d)\n", oldMessage.ID, oldMessage.Thread, threadTS)
				}
			}
			messageCount++
		} else {
			// Skip unknown key types, but log them in verbose mode
			if verbose && (strings.Contains(key, ":msg:") && strings.Contains(key, "01757619432601731301") || strings.Contains(key, "01757963423216750249")) {
				fmt.Printf("⚠️  Skipping malformed key: %s\n", key)
			}
		}
	}

	if verbose {
		fmt.Printf("  Total keys: %d, version:msg: keys: %d\n", totalKeys, versionMsgKeys)
		fmt.Printf("  Extracted %d threads and %d messages\n", threadCount, messageCount)
	}

	// Sort messages within each thread by timestamp
	for threadTS, threadData := range threadDataMap {
		sort.Slice(threadData.Messages, func(i, j int) bool {
			return threadData.Messages[i].TS < threadData.Messages[j].TS
		})
		if verbose {
			fmt.Printf("🔍 Thread %s (TS: %d) has %d messages\n", threadData.Thread.ID, threadTS, len(threadData.Messages))
		}
	}

	return threadDataMap, nil
}

func decryptValue(value []byte, encryptionKey string) ([]byte, error) {
	// Check if data looks like JSON (unencrypted)
	if likelyJSON(value) {
		return value, nil
	}

	// Try to decrypt using old 0.1.2 AES-GCM encryption (full message encryption)
	decrypted, err := decryptWithOldAES(value, encryptionKey)
	if err == nil {
		return decrypted, nil
	}

	// If full decryption fails, try field-level decryption
	return decryptWithFieldEncryption(value, encryptionKey)
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

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func decryptWithFieldEncryption(data []byte, keyHex string) ([]byte, error) {
	// First, try to detect if this is an encrypted envelope
	strValue := string(data)
	if !strings.Contains(strValue, "_enc") {
		// If not an envelope, try regular AES decryption
		return decryptWithOldAES(data, keyHex)
	}

	// This looks like field-level encryption, need to parse JSON and decrypt fields
	var v interface{}
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, fmt.Errorf("failed to unmarshal encrypted envelope: %w", err)
	}

	// Recursively decrypt all fields
	decrypted := decryptAllFields(v, keyHex)

	// Marshal back to JSON
	return json.Marshal(decrypted)
}

func decryptAllFields(node interface{}, keyHex string) interface{} {
	switch cur := node.(type) {
	case map[string]interface{}:
		// Check for envelope directly
		if encType, ok := cur["_enc"].(string); ok {
			if encType == "gcm" {
				if sv, ok := cur["v"].(string); ok {
					if raw, err := base64.StdEncoding.DecodeString(sv); err == nil {
						if pt, err := decryptWithOldAES(raw, keyHex); err == nil {
							// Replace with parsed JSON
							var out interface{}
							if err := json.Unmarshal(pt, &out); err == nil {
								return decryptAllFields(out, keyHex)
							}
						}
					}
				}
			}
		}
		// Recurse into map
		for k, v := range cur {
			cur[k] = decryptAllFields(v, keyHex)
		}
		return cur
	case []interface{}:
		// Recurse into array
		for i, v := range cur {
			cur[i] = decryptAllFields(v, keyHex)
		}
		return cur
	default:
		return node
	}
}

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
			continue
		}

		if err := os.WriteFile(outputFile, threadJSON, 0644); err != nil {
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
				continue
			}

			if err := os.WriteFile(outputFile, messageJSON, 0644); err != nil {
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

func createSystemInfo(timestampedDir string, stats *MigrationStats) error {
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

	return os.WriteFile(filepath.Join(timestampedDir, "system.json"), data, 0644)
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
