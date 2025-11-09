package migrate

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/cockroachdb/pebble"
	"progressdb/clients/cli/config"
)

// Local key generation functions to avoid conflicts
func genThreadKey(threadTS int64) string {
	return fmt.Sprintf("t:%s", PadTS(threadTS))
}

func genMessageKey(threadTS, messageTS int64, seq uint64) string {
	return fmt.Sprintf("t:%s:m:%s:%s", PadTS(threadTS), PadTS(messageTS), PadSeq(seq))
}

func genThreadMessageStart(threadTS string) string {
	return fmt.Sprintf("idx:t:%s:ms:start", threadTS)
}

func genThreadMessageEnd(threadTS string) string {
	return fmt.Sprintf("idx:t:%s:ms:end", threadTS)
}

func genThreadMessageLC(threadTS string) string {
	return fmt.Sprintf("idx:t:%s:ms:lc", threadTS)
}

func genThreadMessageLU(threadTS string) string {
	return fmt.Sprintf("idx:t:%s:ms:lu", threadTS)
}

func genUserOwnsThreadKey(userID, threadTS string) string {
	return fmt.Sprintf("rel:u:%s:t:%s", userID, threadTS)
}

func genThreadHasUserKey(threadTS, userID string) string {
	return fmt.Sprintf("rel:t:%s:u:%s", threadTS, userID)
}

// ensureStateDirs creates the proper directory structure like service/pkg/state/dirs.go
func ensureStateDirs(dbPath string) error {
	storePath := filepath.Join(dbPath, "store")
	walPath := filepath.Join(dbPath, "wal")
	kmsPath := filepath.Join(dbPath, "kms")
	statePath := filepath.Join(dbPath, "state")
	auditPath := filepath.Join(statePath, "audit")
	retentionPath := filepath.Join(statePath, "retention")
	tmpPath := filepath.Join(statePath, "tmp")
	telPath := filepath.Join(statePath, "telemetry")
	logsPath := filepath.Join(statePath, "logs")
	indexPath := filepath.Join(dbPath, "index")

	paths := []string{storePath, walPath, kmsPath, auditPath, retentionPath, tmpPath, telPath, logsPath, indexPath}

	for _, p := range paths {
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			return fmt.Errorf("cannot create parent for %s: %w", p, err)
		}

		if fi, err := os.Lstat(p); err == nil {
			if fi.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("path is a symlink: %s", p)
			}
			if !fi.IsDir() {
				return fmt.Errorf("path exists and is not a directory: %s", p)
			}
		}

		if err := os.MkdirAll(p, 0o700); err != nil {
			return fmt.Errorf("cannot create path %s: %w", p, err)
		}

		if fi2, err := os.Lstat(p); err == nil {
			if fi2.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("path is a symlink after creation: %s", p)
			}
		}

		tmp, err := os.CreateTemp(p, ".validate-*")
		if err != nil {
			return fmt.Errorf("path not writable: %s: %w", p, err)
		}
		tmp.Close()
		_ = os.Remove(tmp.Name())
	}

	return nil
}

// MigrateToPebble migrates data from 0.1.2 to 0.5.0 Pebble format
func MigrateToPebble(cfg *config.Config, verbose bool) error {
	fmt.Printf("🚀 Starting migration from 0.1.2 to 0.5.0 (Pebble output)\n")
	fmt.Printf("📁 Source: %s\n", cfg.FromDatabase)
	fmt.Printf("📁 Target: %s\n", cfg.ToDatabase)

	// Create proper directory structure like service/pkg/state/dirs.go
	if err := ensureStateDirs(cfg.ToDatabase); err != nil {
		return fmt.Errorf("failed to create state directories: %w", err)
	}

	stats := &MigrationStats{}

	// Phase 1: Decrypt and extract data from pebble database
	fmt.Printf("📋 Phase 1: Decrypting and extracting data from pebble database...\n")
	threadDataMap, err := extractAndDecryptDataForPebble(cfg, verbose)
	if err != nil {
		return fmt.Errorf("failed to extract and decrypt data: %w", err)
	}

	fmt.Printf("  Loaded %d threads with their messages\n", len(threadDataMap))

	// Phase 2: Open store and index databases
	fmt.Printf("📋 Phase 2: Opening store and index databases...\n")
	storePath := filepath.Join(cfg.ToDatabase, "store")
	indexPath := filepath.Join(cfg.ToDatabase, "index")

	storeDB, err := pebble.Open(storePath, &pebble.Options{})
	if err != nil {
		return fmt.Errorf("failed to open store database: %w", err)
	}
	defer storeDB.Close()

	indexDB, err := pebble.Open(indexPath, &pebble.Options{})
	if err != nil {
		return fmt.Errorf("failed to open index database: %w", err)
	}
	defer indexDB.Close()

	// Phase 3: Migrate threads and messages to store database
	fmt.Printf("📋 Phase 3: Migrating threads and messages to store database...\n")
	if err := migrateThreadsAndMessagesToStore(threadDataMap, storeDB, stats, verbose); err != nil {
		return fmt.Errorf("thread/message migration failed: %w", err)
	}

	// Phase 4: Create indexes in index database
	fmt.Printf("🗂️ Phase 4: Creating indexes in index database...\n")
	if err := createIndexesInIndexDB(threadDataMap, indexDB, stats, verbose); err != nil {
		return fmt.Errorf("index creation failed: %w", err)
	}

	// Phase 5: Create relationships in index database
	fmt.Printf("🔗 Phase 5: Creating relationships in index database...\n")
	if err := createRelationshipsInIndexDB(threadDataMap, indexDB, stats, verbose); err != nil {
		return fmt.Errorf("relationship creation failed: %w", err)
	}

	// Print final statistics
	fmt.Printf("\n🎉 Migration completed successfully!\n")
	fmt.Printf("📊 Statistics:\n")
	fmt.Printf("  Threads migrated: %d\n", stats.ThreadsMigrated)
	fmt.Printf("  Messages migrated: %d\n", stats.MessagesMigrated)
	fmt.Printf("  Indexes created: %d\n", stats.IndexesCreated)
	fmt.Printf("  Relationships created: %d\n", stats.RelationshipsCreated)

	return nil
}

// migrateThreadsAndMessagesToStore writes threads and messages to store database
func migrateThreadsAndMessagesToStore(threadDataMap map[int64]*ThreadData, storeDB *pebble.DB, stats *MigrationStats, verbose bool) error {
	batch := storeDB.NewBatch()
	defer batch.Close()

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

		// Convert old thread to new format
		newThread := &NewThread{
			Key:       genThreadKey(threadTS),
			Title:     threadData.Thread.Title,
			Author:    threadData.Thread.Author,
			Slug:      threadData.Thread.Slug,
			CreatedTS: threadData.Thread.CreatedTS,
			UpdatedTS: threadData.Thread.UpdatedTS,
			Deleted:   false, // Old threads don't have deleted field
		}

		// Serialize and store thread
		threadJSON, err := json.Marshal(newThread)
		if err != nil {
			return fmt.Errorf("failed to marshal thread %s: %w", threadData.Thread.ID, err)
		}

		if err := batch.Set([]byte(newThread.Key), threadJSON, nil); err != nil {
			return fmt.Errorf("failed to store thread %s: %w", newThread.Key, err)
		}

		if verbose {
			fmt.Printf("  ✅ Migrated thread %s -> %s\n", threadData.Thread.ID, newThread.Key)
		}

		// Sort messages by timestamp
		sort.Slice(threadData.Messages, func(i, j int) bool {
			return threadData.Messages[i].TS < threadData.Messages[j].TS
		})

		// Migrate messages
		for i, oldMessage := range threadData.Messages {
			newMessage := &NewMessage{
				Key:       genMessageKey(threadTS, oldMessage.TS, uint64(i+1)),
				Thread:    newThread.Key,
				Author:    oldMessage.Author,
				Role:      oldMessage.Role,
				CreatedTS: oldMessage.TS,
				UpdatedTS: oldMessage.TS,
				Body:      oldMessage.Body,
				ReplyTo:   oldMessage.ReplyTo,
				Deleted:   oldMessage.Deleted,
			}

			// Serialize and store message
			messageJSON, err := json.Marshal(newMessage)
			if err != nil {
				return fmt.Errorf("failed to marshal message %s: %w", oldMessage.ID, err)
			}

			if err := batch.Set([]byte(newMessage.Key), messageJSON, nil); err != nil {
				return fmt.Errorf("failed to store message %s: %w", newMessage.Key, err)
			}

			stats.MessagesMigrated++
		}

		stats.ThreadsMigrated++

		// Commit batch every 10 threads to avoid large batches
		if stats.ThreadsMigrated%10 == 0 {
			if err := batch.Commit(nil); err != nil {
				return fmt.Errorf("failed to commit batch at thread %d: %w", stats.ThreadsMigrated, err)
			}
			batch = storeDB.NewBatch()
			defer batch.Close()
		}
	}

	// Commit remaining entries
	if err := batch.Commit(nil); err != nil {
		return fmt.Errorf("failed to commit final batch: %w", err)
	}

	return nil
}

// createIndexesInIndexDB creates thread message indexes in index database
func createIndexesInIndexDB(threadDataMap map[int64]*ThreadData, indexDB *pebble.DB, stats *MigrationStats, verbose bool) error {
	batch := indexDB.NewBatch()
	defer batch.Close()

	for threadTS, threadData := range threadDataMap {
		threadKey := genThreadKey(threadTS)

		if len(threadData.Messages) == 0 {
			continue
		}

		// Find first and last message timestamps
		var firstCreatedAt, lastCreatedAt, lastUpdatedAt int64

		for i, msg := range threadData.Messages {
			if i == 0 {
				firstCreatedAt = msg.TS
			}
			if msg.TS > lastCreatedAt {
				lastCreatedAt = msg.TS
			}
			if msg.TS > lastUpdatedAt {
				lastUpdatedAt = msg.TS
			}
		}

		// Create indexes
		indexes := map[string]string{
			genThreadMessageStart(PadTS(threadTS)): "1",                                    // Start sequence
			genThreadMessageEnd(PadTS(threadTS)):   strconv.Itoa(len(threadData.Messages)), // End sequence
			genThreadMessageLC(PadTS(threadTS)):    strconv.FormatInt(firstCreatedAt, 10),  // Last created at
			genThreadMessageLU(PadTS(threadTS)):    strconv.FormatInt(lastUpdatedAt, 10),   // Last updated at
		}

		for indexKey, indexValue := range indexes {
			if err := batch.Set([]byte(indexKey), []byte(indexValue), nil); err != nil {
				return fmt.Errorf("failed to create index %s: %w", indexKey, err)
			}
			stats.IndexesCreated++
		}

		if verbose {
			fmt.Printf("  ✅ Created indexes for thread %s (%d messages)\n", threadKey, len(threadData.Messages))
		}

		// Commit batch every 20 threads
		if stats.IndexesCreated%80 == 0 { // 4 indexes per thread * 20 threads
			if err := batch.Commit(nil); err != nil {
				return fmt.Errorf("failed to commit index batch: %w", err)
			}
			batch = indexDB.NewBatch()
			defer batch.Close()
		}
	}

	// Commit remaining indexes
	if err := batch.Commit(nil); err != nil {
		return fmt.Errorf("failed to commit final index batch: %w", err)
	}

	return nil
}

// createRelationshipsInIndexDB creates user-thread relationships in index database
func createRelationshipsInIndexDB(threadDataMap map[int64]*ThreadData, indexDB *pebble.DB, stats *MigrationStats, verbose bool) error {
	batch := indexDB.NewBatch()
	defer batch.Close()

	for threadTS, threadData := range threadDataMap {
		threadKey := genThreadKey(threadTS)
		userID := threadData.Thread.Author

		// Create user owns thread relationship
		userOwnsThreadKey := genUserOwnsThreadKey(userID, PadTS(threadTS))
		if err := batch.Set([]byte(userOwnsThreadKey), []byte("1"), nil); err != nil {
			return fmt.Errorf("failed to create user owns thread relationship: %w", err)
		}

		// Create thread has user relationship
		threadHasUserKey := genThreadHasUserKey(PadTS(threadTS), userID)
		if err := batch.Set([]byte(threadHasUserKey), []byte("1"), nil); err != nil {
			return fmt.Errorf("failed to create thread has user relationship: %w", err)
		}

		stats.RelationshipsCreated += 2

		if verbose {
			fmt.Printf("  ✅ Created relationships for user %s -> thread %s\n", userID, threadKey)
		}

		// Commit batch every 25 threads
		if stats.RelationshipsCreated%50 == 0 { // 2 relationships per thread * 25 threads
			if err := batch.Commit(nil); err != nil {
				return fmt.Errorf("failed to commit relationship batch: %w", err)
			}
			batch = indexDB.NewBatch()
		}
	}

	// Commit remaining relationships
	if err := batch.Commit(nil); err != nil {
		return fmt.Errorf("failed to commit final relationship batch: %w", err)
	}

	return nil
}

// extractAndDecryptDataForPebble extracts data directly from source database for Pebble migration
func extractAndDecryptDataForPebble(cfg *config.Config, verbose bool) (map[int64]*ThreadData, error) {
	// Open the source pebble database directly
	db, err := pebble.Open(cfg.FromDatabase, &pebble.Options{
		ReadOnly: true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to open source pebble database: %w", err)
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

		// Parse the decrypted JSON value
		var jsonData interface{}
		if err := json.Unmarshal(decryptedValue, &jsonData); err != nil {
			if verbose {
				fmt.Printf("⚠️  Failed to parse JSON for key %s: %v\n", key, err)
			}
			continue
		}

		// Process based on key pattern
		if strings.HasPrefix(key, "thread:") && strings.HasSuffix(key, ":meta") {
			// Thread metadata data
			var thread OldThread
			if err := json.Unmarshal(decryptedValue, &thread); err != nil {
				if verbose {
					fmt.Printf("⚠️  Failed to parse thread data for key %s: %v\n", key, err)
				}
				continue
			}

			// Extract timestamp from thread ID
			threadTS := extractThreadTSFromID(thread.ID)

			if _, exists := threadDataMap[threadTS]; !exists {
				threadDataMap[threadTS] = &ThreadData{
					Thread:   thread,
					Messages: []OldMessage{},
				}
				threadCount++
				if verbose {
					fmt.Printf("✅ Added thread %s with timestamp %d\n", thread.ID, threadTS)
				}
			} else {
				// Update existing thread metadata (important when messages were processed first)
				threadDataMap[threadTS].Thread = thread
				if verbose {
					fmt.Printf("✅ Updated thread metadata for %s with timestamp %d\n", thread.ID, threadTS)
				}
			}

		} else if strings.HasPrefix(key, "version:msg:") {
			// Version message data
			var message OldMessage
			if err := json.Unmarshal(decryptedValue, &message); err != nil {
				if verbose {
					fmt.Printf("⚠️  Failed to parse message data for key %s: %v\n", key, err)
				}
				continue
			}

			// Extract thread timestamp from message thread ID
			threadTS := extractThreadTSFromID(message.Thread)

			if _, exists := threadDataMap[threadTS]; !exists {
				threadDataMap[threadTS] = &ThreadData{
					Thread:   OldThread{}, // Placeholder
					Messages: []OldMessage{},
				}
			}

			threadDataMap[threadTS].Messages = append(threadDataMap[threadTS].Messages, message)
			messageCount++
			if verbose {
				fmt.Printf("✅ Added message %s to thread %d\n", message.ID, threadTS)
			}
		} else if strings.HasPrefix(key, "thread:") && strings.Contains(key, ":msg:") {
			// Thread message data (alternative format)
			var message OldMessage
			if err := json.Unmarshal(decryptedValue, &message); err != nil {
				if verbose {
					fmt.Printf("⚠️  Failed to parse thread message data for key %s: %v\n", key, err)
				}
				continue
			}

			// Extract thread timestamp from message thread ID
			threadTS := extractThreadTSFromID(message.Thread)

			if _, exists := threadDataMap[threadTS]; !exists {
				threadDataMap[threadTS] = &ThreadData{
					Thread:   OldThread{}, // Placeholder
					Messages: []OldMessage{},
				}
			}

			threadDataMap[threadTS].Messages = append(threadDataMap[threadTS].Messages, message)
			messageCount++
			if verbose {
				fmt.Printf("✅ Added thread message %s to thread %d\n", message.ID, threadTS)
			}
		}
	}

	if verbose {
		fmt.Printf("  Processed %d total keys\n", totalKeys)
		fmt.Printf("  Found %d version:msg: keys\n", versionMsgKeys)
		fmt.Printf("  Extracted %d threads and %d messages\n", threadCount, messageCount)
	}

	return threadDataMap, nil
}
