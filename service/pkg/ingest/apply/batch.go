package apply

import (
	"encoding/json"
	"fmt"
	"sort"

	"progressdb/pkg/ingest/queue"
	"progressdb/pkg/ingest/types"
	"progressdb/pkg/logger"
	"progressdb/pkg/models"
	storedb "progressdb/pkg/store/db/store"
	"progressdb/pkg/store/keys"
	"progressdb/pkg/telemetry"
)

// groupByThread groups batch entries by thread ID for scoped processing
func groupByThread(entries []types.BatchEntry) map[string][]types.BatchEntry {
	threadGroups := make(map[string][]types.BatchEntry)

	for _, entry := range entries {
		threadID := entry.TID
		if threadID == "" {
			// Global operations - use empty string as key
			threadID = ""
		}
		threadGroups[threadID] = append(threadGroups[threadID], entry)
	}

	return threadGroups
}

// getOperationPriority returns priority for operation type (CREATE=1, UPDATE=2, DELETE=3)
func getOperationPriority(handler queue.HandlerID) int {
	switch handler {
	case queue.HandlerThreadCreate, queue.HandlerMessageCreate, queue.HandlerReactionAdd:
		return 1 // CREATE
	case queue.HandlerThreadUpdate, queue.HandlerMessageUpdate, queue.HandlerReactionDelete:
		return 2 // UPDATE
	case queue.HandlerThreadDelete, queue.HandlerMessageDelete:
		return 3 // DELETE
	default:
		return 2 // Default to UPDATE
	}
}

// sortOperationsByType sorts entries by operation priority (CREATE → UPDATE → DELETE)
func sortOperationsByType(entries []types.BatchEntry) []types.BatchEntry {
	sorted := make([]types.BatchEntry, len(entries))
	copy(sorted, entries)

	sort.Slice(sorted, func(i, j int) bool {
		priorityI := getOperationPriority(sorted[i].Handler)
		priorityJ := getOperationPriority(sorted[j].Handler)

		// First by priority
		if priorityI != priorityJ {
			return priorityI < priorityJ
		}

		// Then by timestamp for consistent ordering
		return sorted[i].TS < sorted[j].TS
	})

	return sorted
}

// processOperation processes a single operation using BatchIndexManager (ephemeral accumulation)
func processOperation(entry types.BatchEntry, batchIndexManager *BatchIndexManager) error {
	switch entry.Handler {
	case queue.HandlerThreadCreate:
		return processThreadCreate(entry, batchIndexManager)
	case queue.HandlerThreadUpdate:
		return processThreadUpdate(entry, batchIndexManager)
	case queue.HandlerThreadDelete:
		return processThreadDelete(entry, batchIndexManager)
	case queue.HandlerMessageCreate, queue.HandlerMessageUpdate:
		return processMessageSave(entry, batchIndexManager)
	case queue.HandlerMessageDelete:
		return processMessageDelete(entry, batchIndexManager)
	case queue.HandlerReactionAdd, queue.HandlerReactionDelete:
		return processReactionOperation(entry, batchIndexManager)
	default:
		return fmt.Errorf("unknown operation handler: %s", entry.Handler)
	}
}

// processThreadCreate handles thread creation using BatchIndexManager
func processThreadCreate(entry types.BatchEntry, batchIndexManager *BatchIndexManager) error {
	logger.Debug("process_thread_create", "thread", entry.TID)
	// Validate entry
	if entry.TID == "" {
		return fmt.Errorf("thread ID required for thread creation")
	}
	if len(entry.Payload) == 0 {
		return fmt.Errorf("payload required for thread creation")
	}

	// Parse thread model
	var thread models.Thread
	if err := json.Unmarshal(entry.Payload, &thread); err != nil {
		return fmt.Errorf("unmarshal thread: %w", err)
	}

	// Validate thread model
	if thread.ID == "" {
		thread.ID = entry.TID
	}
	if thread.ID == "" {
		return fmt.Errorf("thread ID cannot be empty")
	}
	if thread.CreatedTS == 0 {
		thread.CreatedTS = entry.TS
	}

	// Set thread metadata in batch manager
	batchIndexManager.SetThreadMeta(thread.ID, entry.Payload)

	// Initialize thread message indexes
	batchIndexManager.InitThreadMessageIndexes(thread.ID)

	// Add thread to user ownership and participants
	if thread.Author != "" {
		batchIndexManager.AddThreadToUser(thread.Author, thread.ID)
		batchIndexManager.AddParticipantToThread(thread.ID, thread.Author)
	}

	return nil
}

// processThreadUpdate handles thread updates using BatchIndexManager
func processThreadUpdate(entry types.BatchEntry, batchIndexManager *BatchIndexManager) error {
	logger.Debug("process_thread_update", "thread", entry.TID)
	// Validate entry
	if entry.TID == "" {
		return fmt.Errorf("thread ID required for thread update")
	}
	if len(entry.Payload) == 0 {
		return fmt.Errorf("payload required for thread update")
	}

	// Parse thread model
	var thread models.Thread
	if err := json.Unmarshal(entry.Payload, &thread); err != nil {
		return fmt.Errorf("unmarshal thread: %w", err)
	}

	// Validate thread model
	if thread.ID == "" {
		thread.ID = entry.TID
	}
	if thread.ID == "" {
		return fmt.Errorf("thread ID cannot be empty")
	}

	// Set thread metadata in batch manager
	batchIndexManager.SetThreadMeta(thread.ID, entry.Payload)

	// Update participants if needed
	if thread.Author != "" {
		batchIndexManager.AddParticipantToThread(thread.ID, thread.Author)
	}

	return nil
}

// processThreadDelete handles thread deletion using BatchIndexManager
func processThreadDelete(entry types.BatchEntry, batchIndexManager *BatchIndexManager) error {
	logger.Debug("process_thread_delete", "thread", entry.TID)
	// Validate entry
	if entry.TID == "" {
		return fmt.Errorf("thread ID required for thread deletion")
	}

	// Parse thread model to get author (optional)
	var thread models.Thread
	threadID := entry.TID
	if len(entry.Payload) > 0 {
		if err := json.Unmarshal(entry.Payload, &thread); err != nil {
			return fmt.Errorf("unmarshal thread: %w", err)
		}
		if thread.ID != "" {
			threadID = thread.ID
		}
	}

	if threadID == "" {
		return fmt.Errorf("thread ID cannot be empty")
	}

	// Delete thread metadata from main DB
	batchIndexManager.DeleteThreadMeta(threadID)

	// Delete thread message indexes
	batchIndexManager.DeleteThreadMessageIndexes(threadID)

	// Remove thread from user ownership and add to deleted threads
	if thread.Author != "" {
		batchIndexManager.RemoveThreadFromUser(thread.Author, threadID)
		batchIndexManager.AddDeletedThreadToUser(thread.Author, threadID)
	}

	return nil
}

// processMessageSave handles message save/create operations using BatchIndexManager
func processMessageSave(entry types.BatchEntry, batchIndexManager *BatchIndexManager) error {
	logger.Debug("process_message_save", "thread", entry.TID, "msg", entry.MID, "handler", entry.Handler)
	// Validate entry
	if entry.TID == "" {
		return fmt.Errorf("thread ID required for message save")
	}
	if len(entry.Payload) == 0 && entry.Model == nil {
		return fmt.Errorf("payload or model required for message save")
	}

	// Parse message model
	var msg models.Message
	if entry.Model != nil {
		if m, ok := entry.Model.(*models.Message); ok {
			msg = *m
		}
	} else if len(entry.Payload) > 0 {
		if err := json.Unmarshal(entry.Payload, &msg); err != nil {
			return fmt.Errorf("unmarshal message: %w", err)
		}
	}

	// Validate message model
	if msg.ID == "" {
		return fmt.Errorf("message ID cannot be empty")
	}
	if msg.TS == 0 {
		msg.TS = entry.TS
	}

	// Generate message key and sequence
	msgKey, err := keys.MsgKey(entry.TID, entry.TS, uint64(entry.Enq))
	if err != nil {
		return fmt.Errorf("message key: %w", err)
	}

	// Set message data in batch manager
	if err := batchIndexManager.SetMessageData(entry.TID, msg.ID, entry.Payload, entry.TS, uint64(entry.Enq)); err != nil {
		return fmt.Errorf("set message data: %w", err)
	}

	// Add message version
	if err := batchIndexManager.AddMessageVersion(msg.ID, entry.Payload, entry.TS, uint64(entry.Enq)); err != nil {
		return fmt.Errorf("add message version: %w", err)
	}

	// Update thread message indexes
	isDelete := entry.Handler == queue.HandlerMessageDelete
	batchIndexManager.UpdateThreadMessageIndexes(entry.TID, msg.TS, entry.TS, isDelete, msgKey)

	// Handle user deleted messages
	if isDelete && msg.Author != "" {
		batchIndexManager.AddDeletedMessageToUser(msg.Author, msg.ID)
	}

	return nil
}

// processMessageDelete handles message deletion operations using BatchIndexManager
func processMessageDelete(entry types.BatchEntry, batchIndexManager *BatchIndexManager) error {
	// Parse message model
	var msg models.Message
	if entry.Model != nil {
		if m, ok := entry.Model.(*models.Message); ok {
			msg = *m
		}
	} else if len(entry.Payload) > 0 {
		if err := json.Unmarshal(entry.Payload, &msg); err != nil {
			return fmt.Errorf("unmarshal message: %w", err)
		}
	}

	// Generate message key for deletion tracking
	msgKey, err := keys.MsgKey(entry.TID, entry.TS, uint64(entry.Enq))
	if err != nil {
		return fmt.Errorf("message key: %w", err)
	}

	// Update thread message indexes for deletion
	batchIndexManager.UpdateThreadMessageIndexes(entry.TID, msg.TS, entry.TS, true, msgKey)

	// Add to user's deleted messages
	if msg.Author != "" {
		batchIndexManager.AddDeletedMessageToUser(msg.Author, msg.ID)
	}

	return nil
}

// processReactionOperation handles reaction add/delete operations using BatchIndexManager
func processReactionOperation(entry types.BatchEntry, batchIndexManager *BatchIndexManager) error {
	logger.Debug("process_reaction", "thread", entry.TID, "msg", entry.MID, "handler", entry.Handler)
	// Parse message model
	var msg models.Message
	if entry.Model != nil {
		if m, ok := entry.Model.(*models.Message); ok {
			msg = *m
		}
	} else if len(entry.Payload) > 0 {
		if err := json.Unmarshal(entry.Payload, &msg); err != nil {
			return fmt.Errorf("unmarshal message: %w", err)
		}
	}

	// Generate message key
	msgKey, err := keys.MsgKey(entry.TID, entry.TS, uint64(entry.Enq))
	if err != nil {
		return fmt.Errorf("message key: %w", err)
	}

	// Set updated message data (reactions are merged in payload)
	if err := batchIndexManager.SetMessageData(entry.TID, msg.ID, entry.Payload, entry.TS, uint64(entry.Enq)); err != nil {
		return fmt.Errorf("set message data: %w", err)
	}

	// Add version for reaction change
	if err := batchIndexManager.AddMessageVersion(msg.ID, entry.Payload, entry.TS, uint64(entry.Enq)); err != nil {
		return fmt.Errorf("add message version: %w", err)
	}

	// Update thread indexes
	batchIndexManager.UpdateThreadMessageIndexes(entry.TID, msg.TS, entry.TS, false, msgKey)

	return nil
}

// ApplyBatchToDB persists a list of types.BatchEntry items using optimized batching.
// Groups operations by thread, sorts by type (CREATE→UPDATE→DELETE), processes using BatchIndexManager.
func ApplyBatchToDB(entries []types.BatchEntry) error {
	tr := telemetry.Track("ingest.apply_batch")
	defer tr.Finish()

	if len(entries) == 0 {
		return nil
	}

	logger.Debug("batch_apply_start", "entries", len(entries))

	tr.Mark("group_operations")

	// Create batch index manager for ephemeral accumulation
	batchIndexManager := NewBatchIndexManager()

	// Group entries by thread ID for scoped processing
	threadGroups := groupByThread(entries)
	logger.Debug("batch_grouped", "threads", len(threadGroups))

	tr.Mark("process_thread_groups")

	// Process each thread group
	for threadID, threadEntries := range threadGroups {
		// Sort operations by type (CREATE → UPDATE → DELETE) then by timestamp
		sortedOps := sortOperationsByType(threadEntries)

		logger.Debug("batch_processing_thread", "thread", threadID, "ops", len(sortedOps))

		// Process each operation using BatchIndexManager (ephemeral accumulation)
		for _, op := range sortedOps {
			if err := processOperation(op, batchIndexManager); err != nil {
				logger.Error("process_operation_failed", "err", err, "handler", op.Handler, "thread", op.TID, "msg", op.MID)
				// Continue processing other operations in the thread
				continue
			}
		}
	}

	tr.Mark("flush_batch")

	logger.Debug("batch_flush_start")
	// Single atomic flush for all accumulated changes
	if err := batchIndexManager.Flush(); err != nil {
		logger.Error("batch_flush_failed", "err", err)
		return fmt.Errorf("batch flush failed: %w", err)
	}
	logger.Info("batch_applied", "entries", len(entries))

	tr.Mark("record_write")
	storedb.RecordWrite(len(entries))

	return nil
}
