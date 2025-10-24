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

func groupByThread(entries []types.BatchEntry) map[string][]types.BatchEntry {
	threadGroups := make(map[string][]types.BatchEntry)
	for _, entry := range entries {
		threadID := entry.TID // primary thread identifier

		// If this entry is a thread creation without TID, try to extract it
		if threadID == "" && entry.Handler == queue.HandlerThreadCreate {
			// Try extracting thread ID from the JSON payload
			if len(entry.Payload) > 0 {
				var thread struct {
					ID string `json:"id"`
				}
				if err := json.Unmarshal(entry.Payload, &thread); err == nil && thread.ID != "" {
					threadID = thread.ID
				}
			}
			// Fallback: Try extracting thread ID from the model object
			if threadID == "" && entry.Model != nil {
				if thread, ok := entry.Model.(*models.Thread); ok && thread.ID != "" {
					threadID = thread.ID
				}
			}
		}
		// Group the entry under its threadID (may still be "")
		threadGroups[threadID] = append(threadGroups[threadID], entry)
	}
	return threadGroups
}

func getOperationPriority(handler queue.HandlerID) int {
	switch handler {
	case queue.HandlerThreadCreate, queue.HandlerMessageCreate, queue.HandlerReactionAdd:
		return 1
	case queue.HandlerThreadUpdate, queue.HandlerMessageUpdate, queue.HandlerReactionDelete:
		return 2
	case queue.HandlerThreadDelete, queue.HandlerMessageDelete:
		return 3
	default:
		return 2
	}
}

func sortOperationsByType(entries []types.BatchEntry) []types.BatchEntry {
	sorted := make([]types.BatchEntry, len(entries))
	copy(sorted, entries)
	sort.Slice(sorted, func(i, j int) bool {
		priorityI := getOperationPriority(sorted[i].Handler)
		priorityJ := getOperationPriority(sorted[j].Handler)
		if priorityI != priorityJ {
			return priorityI < priorityJ
		}
		return sorted[i].TS < sorted[j].TS
	})
	return sorted
}

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

func processThreadCreate(entry types.BatchEntry, batchIndexManager *BatchIndexManager) error {
	logger.Info("process_thread_create", "provisional_thread", entry.TID, "ts", entry.TS)
	if len(entry.Payload) == 0 {
		return fmt.Errorf("payload required for thread creation")
	}
	var thread models.Thread
	if err := json.Unmarshal(entry.Payload, &thread); err != nil {
		return fmt.Errorf("unmarshal thread: %w", err)
	}
	if thread.Author == "" {
		return fmt.Errorf("author is required for thread creation")
	}
	threadID := entry.TID
	logger.Debug("thread_using_provisional_as_final", "user", thread.Author, "thread", threadID)
	logger.Debug("thread_create_direct", "thread_key", threadID, "entry_tid", entry.TID)
	batchIndexManager.mu.Lock()
	logger.Debug("mapped_provisional_thread", "provisional", threadID, "final", threadID)
	batchIndexManager.mu.Unlock()
	thread.ID = threadID
	if thread.CreatedTS == 0 {
		thread.CreatedTS = entry.TS
	}
	updatedPayload, err := json.Marshal(thread)
	if err != nil {
		return fmt.Errorf("marshal updated thread: %w", err)
	}
	batchIndexManager.SetThreadMeta(threadID, updatedPayload)
	batchIndexManager.InitThreadMessageIndexes(threadID)
	batchIndexManager.AddThreadToUser(thread.Author, threadID)
	batchIndexManager.AddParticipantToThread(threadID, thread.Author)
	return nil
}

func processThreadDelete(entry types.BatchEntry, batchIndexManager *BatchIndexManager) error {
	logger.Debug("process_thread_delete", "thread", entry.TID)
	if entry.TID == "" {
		return fmt.Errorf("thread ID required for thread deletion")
	}
	batchIndexManager.mu.Lock()
	if keys.ValidateThreadKey(entry.TID) != nil && keys.ValidateThreadPrvKey(entry.TID) != nil {
		return fmt.Errorf("invalid thread key format: %s - expected t:<threadID>", entry.TID)
	}
	finalThreadID := entry.TID
	batchIndexManager.mu.Unlock()
	var thread models.Thread
	threadID := finalThreadID
	if len(entry.Payload) > 0 {
		if err := json.Unmarshal(entry.Payload, &thread); err != nil {
			return fmt.Errorf("unmarshal thread: %w", err)
		}
		if thread.ID != "" {
			threadID = finalThreadID
		}
	}
	if thread.Author == "" {
		return fmt.Errorf("author is required for thread deletion")
	}
	if threadID == "" {
		return fmt.Errorf("thread ID cannot be empty")
	}
	batchIndexManager.DeleteThreadMeta(threadID)
	batchIndexManager.DeleteThreadMessageIndexes(threadID)
	batchIndexManager.RemoveThreadFromUser(thread.Author, threadID)
	batchIndexManager.AddDeletedThreadToUser(thread.Author, threadID)
	return nil
}

func processMessageSave(entry types.BatchEntry, batchIndexManager *BatchIndexManager) error {
	logger.Debug("process_message_save", "thread", entry.TID, "msg", entry.MID, "handler", entry.Handler)
	if entry.TID == "" {
		return fmt.Errorf("thread ID required for message save")
	}
	if len(entry.Payload) == 0 && entry.Model == nil {
		return fmt.Errorf("payload or model required for message save")
	}
	logger.Debug("message_resolve_thread", "provisional_tid", entry.TID, "handler", entry.Handler)
	batchIndexManager.mu.Lock()
	if keys.ValidateThreadKey(entry.TID) != nil && keys.ValidateThreadPrvKey(entry.TID) != nil {
		return fmt.Errorf("invalid thread key format: %s - expected t:<threadID>", entry.TID)
	}
	finalThreadID := entry.TID
	batchIndexManager.mu.Unlock()
	logger.Debug("message_resolved_thread", "provisional", entry.TID, "final", finalThreadID)
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
	var finalMessageID string
	var provisionalMessageID string

	// Use entry.MID as the primary source for the final message ID
	if entry.MID != "" {
		finalMessageID = entry.MID
		logger.Debug("using_entry_mid_as_final", "message", finalMessageID, "mid", entry.MID)
	} else if msg.ID != "" {
		finalMessageID = msg.ID
		logger.Debug("using_msg_id", "message", finalMessageID)
	} else {
		finalMessageID = fmt.Sprintf("%d", entry.TS)
		logger.Debug("fallback_generated_message_id", "message", finalMessageID)
	}

	// If the message has a provisional ID, map it to the final ID
	if msg.ID != "" && batchIndexManager.messageSequencer.IsProvisionalMessageKey(msg.ID) && msg.ID != finalMessageID {
		provisionalMessageID = msg.ID
		batchIndexManager.mu.Lock()
		batchIndexManager.messageSequencer.MapProvisionalToFinalMessageKey(provisionalMessageID, finalMessageID)
		batchIndexManager.mu.Unlock()
		logger.Debug("mapped_provisional_message", "provisional", provisionalMessageID, "final", finalMessageID)
	}
	msg.ID = finalMessageID
	if msg.TS == 0 {
		msg.TS = entry.TS
	}
	msg.Thread = finalThreadID
	updatedPayload, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal updated message: %w", err)
	}
	if err := batchIndexManager.SetMessageData(finalThreadID, finalMessageID, updatedPayload, entry.TS, uint64(entry.Enq)); err != nil {
		return fmt.Errorf("set message data: %w", err)
	}
	if err := batchIndexManager.AddMessageVersion(finalMessageID, updatedPayload, entry.TS, uint64(entry.Enq)); err != nil {
		return fmt.Errorf("add message version: %w", err)
	}
	msgKey := keys.GenMessageKey(finalThreadID, finalMessageID, uint64(entry.Enq))
	isDelete := entry.Handler == queue.HandlerMessageDelete
	batchIndexManager.UpdateThreadMessageIndexes(finalThreadID, msg.TS, entry.TS, isDelete, msgKey)
	if isDelete && msg.Author != "" {
		batchIndexManager.AddDeletedMessageToUser(msg.Author, finalMessageID)
	}
	return nil
}

func processThreadUpdate(entry types.BatchEntry, batchIndexManager *BatchIndexManager) error {
	logger.Debug("process_thread_update", "thread", entry.TID)
	if entry.TID == "" {
		return fmt.Errorf("thread ID required for thread update")
	}
	if len(entry.Payload) == 0 {
		return fmt.Errorf("payload required for thread update")
	}
	logger.Debug("message_resolve_thread", "provisional_tid", entry.TID, "handler", entry.Handler)
	batchIndexManager.mu.Lock()
	if keys.ValidateThreadKey(entry.TID) != nil && keys.ValidateThreadPrvKey(entry.TID) != nil {
		return fmt.Errorf("invalid thread key format: %s - expected t:<threadID>", entry.TID)
	}
	finalThreadID := entry.TID
	batchIndexManager.mu.Unlock()
	logger.Debug("message_resolved_thread", "provisional", entry.TID, "final", finalThreadID)
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
	var finalMessageID string
	var provisionalMessageID string

	// Use entry.MID as the primary source for the final message ID
	if entry.MID != "" {
		finalMessageID = entry.MID
		logger.Debug("using_entry_mid_as_final", "message", finalMessageID, "mid", entry.MID)
	} else if msg.ID != "" {
		finalMessageID = msg.ID
		logger.Debug("using_msg_id", "message", finalMessageID)
	} else {
		finalMessageID = fmt.Sprintf("%d", entry.TS)
		logger.Debug("fallback_generated_message_id", "message", finalMessageID)
	}

	// If the message has a provisional ID, map it to the final ID
	if msg.ID != "" && batchIndexManager.messageSequencer.IsProvisionalMessageKey(msg.ID) && msg.ID != finalMessageID {
		provisionalMessageID = msg.ID
		batchIndexManager.mu.Lock()
		batchIndexManager.messageSequencer.MapProvisionalToFinalMessageKey(provisionalMessageID, finalMessageID)
		batchIndexManager.mu.Unlock()
		logger.Debug("mapped_provisional_message", "provisional", provisionalMessageID, "final", finalMessageID)
	}
	msg.ID = finalMessageID
	if msg.TS == 0 {
		msg.TS = entry.TS
	}
	msg.Thread = finalThreadID
	updatedPayload, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal updated message: %w", err)
	}
	if err := batchIndexManager.SetMessageData(finalThreadID, finalMessageID, updatedPayload, entry.TS, uint64(entry.Enq)); err != nil {
		return fmt.Errorf("set message data: %w", err)
	}
	if err := batchIndexManager.AddMessageVersion(finalMessageID, updatedPayload, entry.TS, uint64(entry.Enq)); err != nil {
		return fmt.Errorf("add message version: %w", err)
	}
	msgKey := keys.GenMessageKey(finalThreadID, finalMessageID, uint64(entry.Enq))
	isDelete := entry.Handler == queue.HandlerMessageDelete
	batchIndexManager.UpdateThreadMessageIndexes(finalThreadID, msg.TS, entry.TS, isDelete, msgKey)
	if isDelete && msg.Author != "" {
		batchIndexManager.AddDeletedMessageToUser(msg.Author, finalMessageID)
	}
	return nil
}

func processMessageDelete(entry types.BatchEntry, batchIndexManager *BatchIndexManager) error {
	if entry.TID == "" {
		return fmt.Errorf("thread ID required for message deletion")
	}
	batchIndexManager.mu.Lock()
	if keys.ValidateThreadKey(entry.TID) != nil && keys.ValidateThreadPrvKey(entry.TID) != nil {
		return fmt.Errorf("invalid thread key format: %s - expected t:<threadID>", entry.TID)
	}
	finalThreadID := entry.TID
	batchIndexManager.mu.Unlock()
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
	batchIndexManager.mu.Lock()
	finalMessageID, err := batchIndexManager.messageSequencer.GetFinalMessageKey(msg.ID)
	batchIndexManager.mu.Unlock()
	if err != nil {
		logger.Error("message_resolution_failed", "provisional_mid", msg.ID, "handler", entry.Handler, "err", err)
		return fmt.Errorf("resolve message ID %s: %w", msg.ID, err)
	}
	msgKey := keys.GenMessageKey(finalThreadID, finalMessageID, uint64(entry.Enq))
	batchIndexManager.UpdateThreadMessageIndexes(finalThreadID, msg.TS, entry.TS, true, msgKey)
	if msg.Author != "" {
		batchIndexManager.AddDeletedMessageToUser(msg.Author, finalMessageID)
	}
	return nil
}

func processReactionOperation(entry types.BatchEntry, batchIndexManager *BatchIndexManager) error {
	logger.Debug("process_reaction", "thread", entry.TID, "msg", entry.MID, "handler", entry.Handler)
	if entry.TID == "" {
		return fmt.Errorf("thread ID required for reaction operation")
	}
	batchIndexManager.mu.Lock()
	if keys.ValidateThreadKey(entry.TID) != nil && keys.ValidateThreadPrvKey(entry.TID) != nil {
		return fmt.Errorf("invalid thread key format: %s - expected t:<threadID>", entry.TID)
	}
	finalThreadID := entry.TID
	batchIndexManager.mu.Unlock()
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
	batchIndexManager.mu.Lock()
	finalMessageID, err := batchIndexManager.messageSequencer.GetFinalMessageKey(msg.ID)
	batchIndexManager.mu.Unlock()
	if err != nil {
		logger.Error("message_resolution_failed", "provisional_mid", msg.ID, "handler", entry.Handler, "err", err)
		return fmt.Errorf("resolve message ID %s: %w", msg.ID, err)
	}
	msg.Thread = finalThreadID
	msg.ID = finalMessageID
	updatedPayload, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal updated message: %w", err)
	}
	if err := batchIndexManager.SetMessageData(finalThreadID, finalMessageID, updatedPayload, entry.TS, uint64(entry.Enq)); err != nil {
		return fmt.Errorf("set message data: %w", err)
	}
	if err := batchIndexManager.AddMessageVersion(finalMessageID, updatedPayload, entry.TS, uint64(entry.Enq)); err != nil {
		return fmt.Errorf("add message version: %w", err)
	}
	msgKey := keys.GenMessageKey(finalThreadID, finalMessageID, uint64(entry.Enq))
	batchIndexManager.UpdateThreadMessageIndexes(finalThreadID, msg.TS, entry.TS, false, msgKey)
	return nil
}

func collectUserIDsFromBatch(entries []types.BatchEntry) []string {
	userMap := make(map[string]bool)
	for _, entry := range entries {
		if entry.Handler == queue.HandlerThreadCreate || entry.Handler == queue.HandlerThreadUpdate || entry.Handler == queue.HandlerThreadDelete {
			if len(entry.Payload) > 0 {
				var thread struct {
					Author string `json:"author"`
				}
				if err := json.Unmarshal(entry.Payload, &thread); err == nil && thread.Author != "" {
					userMap[thread.Author] = true
				}
			}
			if entry.Model != nil {
				if thread, ok := entry.Model.(*models.Thread); ok && thread.Author != "" {
					userMap[thread.Author] = true
				}
			}
		}
		if entry.Handler == queue.HandlerMessageCreate || entry.Handler == queue.HandlerMessageUpdate || entry.Handler == queue.HandlerMessageDelete {
			if len(entry.Payload) > 0 {
				var msg struct {
					Author string `json:"author"`
				}
				if err := json.Unmarshal(entry.Payload, &msg); err == nil && msg.Author != "" {
					userMap[msg.Author] = true
				}
			}
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

func ApplyBatchToDB(entries []types.BatchEntry) error {
	tr := telemetry.Track("ingest.apply_batch")
	defer tr.Finish()
	if len(entries) == 0 {
		return nil
	}
	logger.Debug("batch_apply_start", "entries", len(entries))
	tr.Mark("group_operations")
	batchIndexManager := NewBatchIndexManager()

	// get all users in this batch
	userIDs := collectUserIDsFromBatch(entries)
	if len(userIDs) > 0 {
		tr.Mark("init_user_sequences")
		if err := batchIndexManager.InitializeUserSequencesFromDB(userIDs); err != nil {
			logger.Error("init_user_sequences_failed", "err", err)
		}
	}

	// put reqs into thread groups
	threadGroups := groupByThread(entries)
	logger.Debug("batch_grouped", "threads", len(threadGroups))
	for threadID, threadEntries := range threadGroups {
		logger.Debug("thread_group", "thread_id", threadID, "operations", len(threadEntries))
		for _, entry := range threadEntries {
			logger.Debug("thread_group_op", "thread_id", threadID, "handler", entry.Handler, "tid", entry.TID, "mid", entry.MID)
		}
	}
	tr.Mark("process_thread_groups")

	for threadID, threadEntries := range threadGroups {
		sortedOps := sortOperationsByType(threadEntries)
		logger.Debug("batch_processing_thread", "thread", threadID, "ops", len(sortedOps))
		for _, op := range sortedOps {
			if err := processOperation(op, batchIndexManager); err != nil {
				logger.Error("process_operation_failed", "err", err, "handler", op.Handler, "thread", op.TID, "msg", op.MID)
				continue
			}
		}
	}
	tr.Mark("flush_batch")
	logger.Debug("batch_flush_start")
	if err := batchIndexManager.Flush(); err != nil {
		logger.Error("batch_flush_failed", "err", err)
		return fmt.Errorf("batch flush failed: %w", err)
	}
	logger.Info("batch_applied", "entries", len(entries))
	tr.Mark("record_write")
	storedb.RecordWrite(len(entries))
	return nil
}
