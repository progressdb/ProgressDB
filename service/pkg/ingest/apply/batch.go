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

		// For thread creation operations, the TID should already be the provisional ID
		// But if it's empty, try to extract from payload (backward compatibility)
		if threadID == "" && entry.Handler == queue.HandlerThreadCreate {
			if len(entry.Payload) > 0 {
				var thread struct {
					ID string `json:"id"`
				}
				if err := json.Unmarshal(entry.Payload, &thread); err == nil && thread.ID != "" {
					threadID = thread.ID
				}
			}
			// Also check Model field
			if threadID == "" && entry.Model != nil {
				if thread, ok := entry.Model.(*models.Thread); ok && thread.ID != "" {
					threadID = thread.ID
				}
			}
		}

		// Group by thread ID (provisional or final)
		// Thread creation and related messages will have the same provisional ID
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
	logger.Info("process_thread_create", "provisional_thread", entry.TID, "ts", entry.TS)

	// Validate entry - thread ID is not required for creation as it will be generated
	if len(entry.Payload) == 0 {
		return fmt.Errorf("payload required for thread creation")
	}

	// Parse thread model
	var thread models.Thread
	if err := json.Unmarshal(entry.Payload, &thread); err != nil {
		return fmt.Errorf("unmarshal thread: %w", err)
	}

	// Validate author is required for thread creation
	if thread.Author == "" {
		return fmt.Errorf("author is required for thread creation")
	}

	// Use the provisional ID as the final thread ID since they're now identical
	threadID := entry.TID
	logger.Debug("thread_using_provisional_as_final", "user", thread.Author, "thread", threadID)

	// No mapping needed since provisional == final
	logger.Debug("thread_create_direct", "thread_key", threadID, "entry_tid", entry.TID)

	// No mapping needed since provisional == final
	batchIndexManager.mu.Lock()
	batchIndexManager.sequencerManager.MapProvisionalToFinalThreadID(threadID, threadID)
	batchIndexManager.mu.Unlock()

	// Update thread model with generated ID
	thread.ID = threadID
	if thread.CreatedTS == 0 {
		thread.CreatedTS = entry.TS
	}

	// Re-marshal the updated thread model
	updatedPayload, err := json.Marshal(thread)
	if err != nil {
		return fmt.Errorf("marshal updated thread: %w", err)
	}

	// Set thread metadata in batch manager
	batchIndexManager.SetThreadMeta(threadID, updatedPayload)

	// Initialize thread message indexes
	batchIndexManager.InitThreadMessageIndexes(threadID)

	// Add thread to user ownership and participants
	batchIndexManager.AddThreadToUser(thread.Author, threadID)
	batchIndexManager.AddParticipantToThread(threadID, thread.Author)

	return nil
}

// processThreadUpdate handles thread updates using BatchIndexManager
// processThreadDelete handles thread deletion using BatchIndexManager
func processThreadDelete(entry types.BatchEntry, batchIndexManager *BatchIndexManager) error {
	logger.Debug("process_thread_delete", "thread", entry.TID)
	// Validate entry
	if entry.TID == "" {
		return fmt.Errorf("thread ID required for thread deletion")
	}

	// Resolve thread ID (handles provisional → final ID conversion)
	batchIndexManager.mu.Lock()
	finalThreadID, err := batchIndexManager.sequencerManager.GetFinalThreadID(entry.TID)
	batchIndexManager.mu.Unlock()
	if err != nil {
		logger.Error("thread_resolution_failed", "provisional_tid", entry.TID, "handler", entry.Handler, "err", err)
		return fmt.Errorf("resolve thread ID %s: %w", entry.TID, err)
	}

	// Parse thread model to get author
	var thread models.Thread
	threadID := finalThreadID
	if len(entry.Payload) > 0 {
		if err := json.Unmarshal(entry.Payload, &thread); err != nil {
			return fmt.Errorf("unmarshal thread: %w", err)
		}
		if thread.ID != "" {
			// Use the final ID from resolution, not the one from payload
			threadID = finalThreadID
		}
	}

	// Validate author is required for thread deletion
	if thread.Author == "" {
		return fmt.Errorf("author is required for thread deletion")
	}

	if threadID == "" {
		return fmt.Errorf("thread ID cannot be empty")
	}

	// Delete thread metadata from main DB
	batchIndexManager.DeleteThreadMeta(threadID)

	// Delete thread message indexes
	batchIndexManager.DeleteThreadMessageIndexes(threadID)

	// Remove thread from user ownership and add to deleted threads
	batchIndexManager.RemoveThreadFromUser(thread.Author, threadID)
	batchIndexManager.AddDeletedThreadToUser(thread.Author, threadID)

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

	// Resolve thread ID (handles provisional → final ID conversion)
	logger.Debug("message_resolve_thread", "provisional_tid", entry.TID, "handler", entry.Handler)
	batchIndexManager.mu.Lock()
	finalThreadID, err := batchIndexManager.sequencerManager.GetFinalThreadID(entry.TID)
	batchIndexManager.mu.Unlock()
	if err != nil {
		logger.Error("thread_resolution_failed", "provisional_tid", entry.TID, "handler", entry.Handler, "err", err)
		return fmt.Errorf("resolve thread ID %s: %w", entry.TID, err)
	}
	logger.Debug("message_resolved_thread", "provisional", entry.TID, "final", finalThreadID)

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

	// Handle message ID generation and mapping
	var finalMessageID string
	var provisionalMessageID string

	if msg.ID == "" {
		// Generate new message ID
		finalMessageID = fmt.Sprintf("msg-%d", entry.TS)
		logger.Debug("generated_message_id", "message", finalMessageID)
	} else if batchIndexManager.sequencerManager.IsProvisionalMessageID(msg.ID) {
		// This is a provisional ID, generate final ID and map it
		provisionalMessageID = msg.ID
		finalMessageID = fmt.Sprintf("msg-%d", entry.TS)
		batchIndexManager.mu.Lock()
		batchIndexManager.sequencerManager.MapProvisionalToFinalMessageID(provisionalMessageID, finalMessageID)
		batchIndexManager.mu.Unlock()
		logger.Debug("mapped_provisional_message", "provisional", provisionalMessageID, "final", finalMessageID)
	} else {
		// This is already a final ID
		finalMessageID = msg.ID
	}

	// Update message model with final ID
	msg.ID = finalMessageID
	if msg.TS == 0 {
		msg.TS = entry.TS
	}

	// Update message thread reference to final ID
	msg.Thread = finalThreadID

	// Re-marshal the updated message model with final IDs
	updatedPayload, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal updated message: %w", err)
	}

	// Use updated payload for storage
	if err := batchIndexManager.SetMessageData(finalThreadID, finalMessageID, updatedPayload, entry.TS, uint64(entry.Enq)); err != nil {
		return fmt.Errorf("set message data: %w", err)
	}
	if err := batchIndexManager.AddMessageVersion(finalMessageID, updatedPayload, entry.TS, uint64(entry.Enq)); err != nil {
		return fmt.Errorf("add message version: %w", err)
	}

	// Generate message key and sequence
	msgKey := keys.GenMessageKey(finalThreadID, finalMessageID, uint64(entry.Enq))

	// Update thread message indexes
	isDelete := entry.Handler == queue.HandlerMessageDelete
	batchIndexManager.UpdateThreadMessageIndexes(finalThreadID, msg.TS, entry.TS, isDelete, msgKey)

	// Handle user deleted messages
	if isDelete && msg.Author != "" {
		batchIndexManager.AddDeletedMessageToUser(msg.Author, finalMessageID)
	}

	return nil
}

func processThreadUpdate(entry types.BatchEntry, batchIndexManager *BatchIndexManager) error {
	logger.Debug("process_thread_update", "thread", entry.TID)
	// Validate entry
	if entry.TID == "" {
		return fmt.Errorf("thread ID required for thread update")
	}
	if len(entry.Payload) == 0 {
		return fmt.Errorf("payload required for thread update")
	}

	// Resolve thread ID (handles provisional → final ID conversion)
	logger.Debug("message_resolve_thread", "provisional_tid", entry.TID, "handler", entry.Handler)
	batchIndexManager.mu.Lock()
	finalThreadID, err := batchIndexManager.sequencerManager.GetFinalThreadID(entry.TID)
	batchIndexManager.mu.Unlock()
	if err != nil {
		logger.Error("thread_resolution_failed", "provisional_tid", entry.TID, "handler", entry.Handler, "err", err)
		return fmt.Errorf("resolve thread ID %s: %w", entry.TID, err)
	}
	logger.Debug("message_resolved_thread", "provisional", entry.TID, "final", finalThreadID)

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

	// Handle message ID generation and mapping
	var finalMessageID string
	var provisionalMessageID string

	if msg.ID == "" {
		// Generate new message ID
		finalMessageID = fmt.Sprintf("msg-%d", entry.TS)
		logger.Debug("generated_message_id", "message", finalMessageID)
	} else if batchIndexManager.sequencerManager.IsProvisionalMessageID(msg.ID) {
		// This is a provisional ID, generate final ID and map it
		provisionalMessageID = msg.ID
		finalMessageID = fmt.Sprintf("msg-%d", entry.TS)
		batchIndexManager.mu.Lock()
		batchIndexManager.sequencerManager.MapProvisionalToFinalMessageID(provisionalMessageID, finalMessageID)
		batchIndexManager.mu.Unlock()
		logger.Debug("mapped_provisional_message", "provisional", provisionalMessageID, "final", finalMessageID)
	} else {
		// This is already a final ID
		finalMessageID = msg.ID
	}

	// Update message model with final ID
	msg.ID = finalMessageID
	if msg.TS == 0 {
		msg.TS = entry.TS
	}

	// Update message thread reference to final ID
	msg.Thread = finalThreadID

	// Re-marshal the updated message model with final IDs
	updatedPayload, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal updated message: %w", err)
	}

	// Use updated payload for storage
	if err := batchIndexManager.SetMessageData(finalThreadID, finalMessageID, updatedPayload, entry.TS, uint64(entry.Enq)); err != nil {
		return fmt.Errorf("set message data: %w", err)
	}
	if err := batchIndexManager.AddMessageVersion(finalMessageID, updatedPayload, entry.TS, uint64(entry.Enq)); err != nil {
		return fmt.Errorf("add message version: %w", err)
	}

	// Generate message key and sequence
	msgKey := keys.GenMessageKey(finalThreadID, finalMessageID, uint64(entry.Enq))

	// Update thread message indexes
	isDelete := entry.Handler == queue.HandlerMessageDelete
	batchIndexManager.UpdateThreadMessageIndexes(finalThreadID, msg.TS, entry.TS, isDelete, msgKey)

	// Handle user deleted messages
	if isDelete && msg.Author != "" {
		batchIndexManager.AddDeletedMessageToUser(msg.Author, finalMessageID)
	}

	return nil
}

// processMessageDelete handles message deletion operations using BatchIndexManager
func processMessageDelete(entry types.BatchEntry, batchIndexManager *BatchIndexManager) error {
	// Validate entry
	if entry.TID == "" {
		return fmt.Errorf("thread ID required for message deletion")
	}

	// Resolve thread ID (handles provisional → final ID conversion)
	batchIndexManager.mu.Lock()
	finalThreadID, err := batchIndexManager.sequencerManager.GetFinalThreadID(entry.TID)
	batchIndexManager.mu.Unlock()
	if err != nil {
		logger.Error("thread_resolution_failed", "provisional_tid", entry.TID, "handler", entry.Handler, "err", err)
		return fmt.Errorf("resolve thread ID %s: %w", entry.TID, err)
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

	// Resolve message ID (handles provisional → final ID conversion)
	batchIndexManager.mu.Lock()
	finalMessageID, err := batchIndexManager.sequencerManager.GetFinalMessageID(msg.ID)
	batchIndexManager.mu.Unlock()
	if err != nil {
		logger.Error("message_resolution_failed", "provisional_mid", msg.ID, "handler", entry.Handler, "err", err)
		return fmt.Errorf("resolve message ID %s: %w", msg.ID, err)
	}

	// Generate message key for deletion tracking
	msgKey := keys.GenMessageKey(finalThreadID, finalMessageID, uint64(entry.Enq))

	// Update thread message indexes for deletion
	batchIndexManager.UpdateThreadMessageIndexes(finalThreadID, msg.TS, entry.TS, true, msgKey)

	// Add to user's deleted messages
	if msg.Author != "" {
		batchIndexManager.AddDeletedMessageToUser(msg.Author, finalMessageID)
	}

	return nil
}

// processReactionOperation handles reaction add/delete operations using BatchIndexManager
func processReactionOperation(entry types.BatchEntry, batchIndexManager *BatchIndexManager) error {
	logger.Debug("process_reaction", "thread", entry.TID, "msg", entry.MID, "handler", entry.Handler)

	// Validate entry
	if entry.TID == "" {
		return fmt.Errorf("thread ID required for reaction operation")
	}

	// Resolve thread ID (handles provisional → final ID conversion)
	batchIndexManager.mu.Lock()
	finalThreadID, err := batchIndexManager.sequencerManager.GetFinalThreadID(entry.TID)
	batchIndexManager.mu.Unlock()
	if err != nil {
		logger.Error("thread_resolution_failed", "provisional_tid", entry.TID, "handler", entry.Handler, "err", err)
		return fmt.Errorf("resolve thread ID %s: %w", entry.TID, err)
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

	// Resolve message ID (handles provisional → final ID conversion)
	batchIndexManager.mu.Lock()
	finalMessageID, err := batchIndexManager.sequencerManager.GetFinalMessageID(msg.ID)
	batchIndexManager.mu.Unlock()
	if err != nil {
		logger.Error("message_resolution_failed", "provisional_mid", msg.ID, "handler", entry.Handler, "err", err)
		return fmt.Errorf("resolve message ID %s: %w", msg.ID, err)
	}

	// Update message thread reference to final ID
	msg.Thread = finalThreadID
	msg.ID = finalMessageID

	// Re-marshal the updated message model with final IDs
	updatedPayload, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal updated message: %w", err)
	}

	// Use updated payload for storage
	if err := batchIndexManager.SetMessageData(finalThreadID, finalMessageID, updatedPayload, entry.TS, uint64(entry.Enq)); err != nil {
		return fmt.Errorf("set message data: %w", err)
	}
	if err := batchIndexManager.AddMessageVersion(finalMessageID, updatedPayload, entry.TS, uint64(entry.Enq)); err != nil {
		return fmt.Errorf("add message version: %w", err)
	}

	// Generate message key and sequence
	msgKey := keys.GenMessageKey(finalThreadID, finalMessageID, uint64(entry.Enq))

	// Update thread indexes
	batchIndexManager.UpdateThreadMessageIndexes(finalThreadID, msg.TS, entry.TS, false, msgKey)

	return nil
}

// collectUserIDsFromBatch extracts all unique user IDs from batch entries
func collectUserIDsFromBatch(entries []types.BatchEntry) []string {
	userMap := make(map[string]bool)
	for _, entry := range entries {
		// Extract user ID from thread payloads for thread operations
		if entry.Handler == queue.HandlerThreadCreate || entry.Handler == queue.HandlerThreadUpdate || entry.Handler == queue.HandlerThreadDelete {
			if len(entry.Payload) > 0 {
				var thread struct {
					Author string `json:"author"`
				}
				if err := json.Unmarshal(entry.Payload, &thread); err == nil && thread.Author != "" {
					userMap[thread.Author] = true
				}
			}
			// Also check Model field for thread operations
			if entry.Model != nil {
				if thread, ok := entry.Model.(*models.Thread); ok && thread.Author != "" {
					userMap[thread.Author] = true
				}
			}
		}
		// Extract user ID from message payloads for message operations
		if entry.Handler == queue.HandlerMessageCreate || entry.Handler == queue.HandlerMessageUpdate || entry.Handler == queue.HandlerMessageDelete {
			if len(entry.Payload) > 0 {
				var msg struct {
					Author string `json:"author"`
				}
				if err := json.Unmarshal(entry.Payload, &msg); err == nil && msg.Author != "" {
					userMap[msg.Author] = true
				}
			}
			// Also check Model field for message operations
			if entry.Model != nil {
				if msg, ok := entry.Model.(*models.Message); ok && msg.Author != "" {
					userMap[msg.Author] = true
				}
			}
		}
	}

	userIDs := make([]string, 0, len(userMap))
	for userID := range userMap {
		userIDs = append(userIDs, userID)
	}
	return userIDs
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

	// Initialize user thread sequences from database
	userIDs := collectUserIDsFromBatch(entries)
	if len(userIDs) > 0 {
		tr.Mark("init_user_sequences")
		if err := batchIndexManager.InitializeUserSequencesFromDB(userIDs); err != nil {
			logger.Error("init_user_sequences_failed", "err", err)
			// Continue processing - sequences will be initialized lazily
		}
	}

	// Group entries by thread ID for scoped processing
	threadGroups := groupByThread(entries)
	logger.Debug("batch_grouped", "threads", len(threadGroups))

	// Debug: Log thread groups and their operations
	for threadID, threadEntries := range threadGroups {
		logger.Debug("thread_group", "thread_id", threadID, "operations", len(threadEntries))
		for _, entry := range threadEntries {
			logger.Debug("thread_group_op", "thread_id", threadID, "handler", entry.Handler, "tid", entry.TID, "mid", entry.MID)
		}
	}

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
