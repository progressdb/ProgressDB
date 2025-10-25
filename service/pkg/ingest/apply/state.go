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
	// Note: Ownership and participants now use immediate key-based markers, so no need to track userOwnership/threadParticipants

	// Create batches AFTER all reads are complete
	mainBatch := storedb.Client.NewBatch()
	indexBatch := index.IndexDB.NewBatch()

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

	if len(errors) > 0 {
		for _, err := range errors {
			logger.Error("batch_flush_error", "err", err)
		}
		mainBatch.Close()
		indexBatch.Close()
		return fmt.Errorf("batch flush completed with %d errors", len(errors))
	}

	// Commit batches
	if err := mainBatch.Commit(storedb.WriteOpt(true)); err != nil {
		logger.Error("main_batch_commit_error", "err", err)
		mainBatch.Close()
		indexBatch.Close()
		return fmt.Errorf("commit main batch: %w", err)
	}
	if err := indexBatch.Commit(index.WriteOpt(true)); err != nil {
		logger.Error("index_batch_commit_error", "err", err)
		mainBatch.Close()
		indexBatch.Close()
		return fmt.Errorf("commit index batch: %w", err)
	}

	// Close batches after successful commit
	mainBatch.Close()
	indexBatch.Close()

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
