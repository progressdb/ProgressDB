package apply

import (
	"encoding/json"
	"fmt"

	"progressdb/pkg/logger"
	"progressdb/pkg/store/db/index"
	storedb "progressdb/pkg/store/db/store"
	"progressdb/pkg/store/keys"
)

// BatchProcessor coordinates index and data managers for batch processing
type BatchProcessor struct {
	Index *IndexManager // Public field access
	Data  *DataManager  // Public field access
}

// NewBatchProcessor creates a new batch processor with initialized managers
func NewBatchProcessor() *BatchProcessor {
	return &BatchProcessor{
		Index: NewIndexManager(),
		Data:  NewDataManager(),
	}
}

// Flush writes all accumulated changes to databases
func (bp *BatchProcessor) Flush() error {
	// Get current states from managers
	threadMeta := bp.Data.GetThreadMeta()
	messageData := bp.Data.GetMessageData()
	versionKeys := bp.Data.GetVersionKeys()
	threadMessages := bp.Index.GetThreadMessages()
	indexData := bp.Index.GetIndexData()
	userOwnership := bp.Index.GetUserOwnership()
	threadParticipants := bp.Index.GetThreadParticipants()
	deletedThreads := bp.Index.GetSoftDeletedThreads()
	deletedMessages := bp.Index.GetSoftDeletedMessages()

	// Create batches
	mainBatch := storedb.Client.NewBatch()
	indexBatch := index.IndexDB.NewBatch()
	defer mainBatch.Close()
	defer indexBatch.Close()

	var errors []error

	// Write thread meta to main DB
	for threadID, data := range threadMeta {
		threadKey := keys.GenThreadKey(threadID)
		if data == nil {
			if err := mainBatch.Delete([]byte(threadKey), storedb.WriteOpt(true)); err != nil {
				errors = append(errors, fmt.Errorf("delete thread meta %s: %w", threadID, err))
			}
		} else {
			if err := mainBatch.Set([]byte(threadKey), data, storedb.WriteOpt(true)); err != nil {
				errors = append(errors, fmt.Errorf("set thread meta %s: %w", threadID, err))
			}
		}
	}

	// Write message data to main DB
	for key, msgData := range messageData {
		if err := mainBatch.Set([]byte(key), msgData.Data, storedb.WriteOpt(true)); err != nil {
			errors = append(errors, fmt.Errorf("set message data %s: %w", key, err))
		}
	}

	// Write version keys to index DB
	for versionKey, versionData := range versionKeys {
		if err := indexBatch.Set([]byte(versionKey), versionData, index.WriteOpt(true)); err != nil {
			errors = append(errors, fmt.Errorf("set version key %s: %w", versionKey, err))
		}
	}

	// Write thread indexes to index DB
	for threadID, threadIdx := range threadMessages {
		threadKey := keys.GenThreadMessageStart(threadID)
		if threadIdx == nil {
			if err := indexBatch.Delete([]byte(threadKey), index.WriteOpt(true)); err != nil {
				errors = append(errors, fmt.Errorf("delete thread messages %s: %w", threadID, err))
			}
		} else {
			data, err := json.Marshal(threadIdx)
			if err != nil {
				errors = append(errors, fmt.Errorf("marshal thread messages %s: %w", threadID, err))
				continue
			}
			if err := indexBatch.Set([]byte(threadKey), data, index.WriteOpt(true)); err != nil {
				errors = append(errors, fmt.Errorf("set thread messages %s: %w", threadID, err))
			}
		}
	}

	// Write index data to index DB
	for key, data := range indexData {
		if err := indexBatch.Set([]byte(key), data, index.WriteOpt(true)); err != nil {
			errors = append(errors, fmt.Errorf("set index data %s: %w", key, err))
		}
	}

	// Apply both batches atomically
	if len(errors) == 0 {
		if err := storedb.ApplyBatch(mainBatch, true); err != nil {
			errors = append(errors, fmt.Errorf("apply main batch: %w", err))
		} else {
			logger.Debug("batch_main_synced")
		}
		if err := storedb.ApplyIndexBatch(indexBatch, true); err != nil {
			errors = append(errors, fmt.Errorf("apply index batch: %w", err))
		} else {
			logger.Debug("batch_index_synced")
		}
	}

	// Add user ownership changes to index batch
	for userID, threads := range userOwnership {
		userThreadsKey := keys.GenUserThreadsKey(userID)

		// Load existing user threads
		var existingThreads []string
		if val, err := index.GetKey(userThreadsKey); err == nil && val != "" {
			var indexes index.UserThreadIndexes
			if err := json.Unmarshal([]byte(val), &indexes); err == nil {
				existingThreads = indexes.Threads
			}
		}

		// Apply changes
		var updatedThreads []string
		threadSet := make(map[string]bool)
		for _, threadID := range existingThreads {
			threadSet[threadID] = true
		}

		for threadID, owns := range threads {
			if owns {
				threadSet[threadID] = true
			} else {
				delete(threadSet, threadID)
			}
		}

		for threadID := range threadSet {
			updatedThreads = append(updatedThreads, threadID)
		}

		// Add to batch
		if len(updatedThreads) == 0 {
			if err := indexBatch.Delete([]byte(userThreadsKey), index.WriteOpt(true)); err != nil {
				errors = append(errors, fmt.Errorf("delete user threads %s: %w", userID, err))
			}
		} else {
			data, err := json.Marshal(index.UserThreadIndexes{Threads: updatedThreads})
			if err != nil {
				errors = append(errors, fmt.Errorf("marshal user threads %s: %w", userID, err))
				continue
			}
			if err := indexBatch.Set([]byte(userThreadsKey), data, index.WriteOpt(true)); err != nil {
				errors = append(errors, fmt.Errorf("set user threads %s: %w", userID, err))
			}
		}
	}

	// Add thread participants changes to index batch
	for threadID, users := range threadParticipants {
		participantsKey := keys.GenThreadParticipantsKey(threadID)

		// Load existing participants
		var existingParticipants []string
		if val, err := index.GetKey(participantsKey); err == nil && val != "" {
			var indexes index.ThreadParticipantIndexes
			if err := json.Unmarshal([]byte(val), &indexes); err == nil {
				existingParticipants = indexes.Participants
			}
		}

		// Apply changes
		var updatedParticipants []string
		userSet := make(map[string]bool)
		for _, userID := range existingParticipants {
			userSet[userID] = true
		}

		for userID, participates := range users {
			if participates {
				userSet[userID] = true
			} else {
				delete(userSet, userID)
			}
		}

		for userID := range userSet {
			updatedParticipants = append(updatedParticipants, userID)
		}

		// Add to batch
		if len(updatedParticipants) == 0 {
			if err := indexBatch.Delete([]byte(participantsKey), index.WriteOpt(true)); err != nil {
				errors = append(errors, fmt.Errorf("delete thread participants %s: %w", threadID, err))
			}
		} else {
			data, err := json.Marshal(index.ThreadParticipantIndexes{Participants: updatedParticipants})
			if err != nil {
				errors = append(errors, fmt.Errorf("marshal thread participants %s: %w", threadID, err))
				continue
			}
			if err := indexBatch.Set([]byte(participantsKey), data, index.WriteOpt(true)); err != nil {
				errors = append(errors, fmt.Errorf("set thread participants %s: %w", threadID, err))
			}
		}
	}

	// Add deleted threads changes to index batch
	for userID, threads := range deletedThreads {
		deletedThreadsKey := keys.GenSoftDeletedThreadsKey(userID)

		// Load existing deleted threads
		var existingDeletedThreads []string
		if val, err := index.GetKey(deletedThreadsKey); err == nil && val != "" {
			var indexes index.UserSoftDeletedThreads
			if err := json.Unmarshal([]byte(val), &indexes); err == nil {
				existingDeletedThreads = indexes.Threads
			}
		}

		// Apply changes
		var updatedDeletedThreads []string
		threadSet := make(map[string]bool)
		for _, threadID := range existingDeletedThreads {
			threadSet[threadID] = true
		}

		for threadID, deleted := range threads {
			if deleted {
				threadSet[threadID] = true
			} else {
				delete(threadSet, threadID)
			}
		}

		for threadID := range threadSet {
			updatedDeletedThreads = append(updatedDeletedThreads, threadID)
		}

		// Add to batch
		if len(updatedDeletedThreads) == 0 {
			if err := indexBatch.Delete([]byte(deletedThreadsKey), index.WriteOpt(true)); err != nil {
				errors = append(errors, fmt.Errorf("delete deleted threads %s: %w", userID, err))
			}
		} else {
			data, err := json.Marshal(index.UserSoftDeletedThreads{Threads: updatedDeletedThreads})
			if err != nil {
				errors = append(errors, fmt.Errorf("marshal deleted threads %s: %w", userID, err))
				continue
			}
			if err := indexBatch.Set([]byte(deletedThreadsKey), data, index.WriteOpt(true)); err != nil {
				errors = append(errors, fmt.Errorf("set deleted threads %s: %w", userID, err))
			}
		}
	}

	// Add deleted messages changes to index batch
	for userID, messages := range deletedMessages {
		deletedMessagesKey := keys.GenSoftDeletedMessagesKey(userID)

		// Load existing deleted messages
		var existingDeletedMessages []string
		if val, err := index.GetKey(deletedMessagesKey); err == nil && val != "" {
			var indexes index.UserSoftDeletedMessages
			if err := json.Unmarshal([]byte(val), &indexes); err == nil {
				existingDeletedMessages = indexes.Messages
			}
		}

		// Apply changes
		var updatedDeletedMessages []string
		messageSet := make(map[string]bool)
		for _, messageID := range existingDeletedMessages {
			messageSet[messageID] = true
		}

		for messageID, deleted := range messages {
			if deleted {
				messageSet[messageID] = true
			} else {
				delete(messageSet, messageID)
			}
		}

		for messageID := range messageSet {
			updatedDeletedMessages = append(updatedDeletedMessages, messageID)
		}

		// Add to batch
		if len(updatedDeletedMessages) == 0 {
			if err := indexBatch.Delete([]byte(deletedMessagesKey), index.WriteOpt(true)); err != nil {
				errors = append(errors, fmt.Errorf("delete deleted messages %s: %w", userID, err))
			}
		} else {
			data, err := json.Marshal(index.UserSoftDeletedMessages{Messages: updatedDeletedMessages})
			if err != nil {
				errors = append(errors, fmt.Errorf("marshal deleted messages %s: %w", userID, err))
				continue
			}
			if err := indexBatch.Set([]byte(deletedMessagesKey), data, index.WriteOpt(true)); err != nil {
				errors = append(errors, fmt.Errorf("set deleted messages %s: %w", userID, err))
			}
		}
	}

	if len(errors) > 0 {
		for _, err := range errors {
			logger.Error("batch_flush_error", "err", err)
		}
		return fmt.Errorf("batch flush completed with %d errors", len(errors))
	}

	// Reset after successful flush
	bp.Reset()
	logger.Debug("batch_reset_complete")

	return nil
}

// Reset clears all accumulated changes after batch completion
func (bp *BatchProcessor) Reset() {
	bp.Data.Reset()
	bp.Index.Reset()
}
