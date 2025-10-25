package apply

import (
	"encoding/json"
	"fmt"

	"progressdb/pkg/ingest/queue"
	"progressdb/pkg/ingest/types"
	"progressdb/pkg/logger"
	"progressdb/pkg/models"
	"progressdb/pkg/store/keys"
)

func processOperation(entry types.BatchEntry, batchProcessor *BatchProcessor) error {
	switch entry.Handler {
	case queue.HandlerThreadCreate:
		return processThreadCreate(entry, batchProcessor)
	case queue.HandlerThreadUpdate:
		return processThreadUpdate(entry, batchProcessor)
	case queue.HandlerThreadDelete:
		return processThreadDelete(entry, batchProcessor)
	case queue.HandlerMessageCreate:
		return processMessageCreate(entry, batchProcessor)
	case queue.HandlerMessageUpdate:
		return processMessageUpdate(entry, batchProcessor)
	case queue.HandlerMessageDelete:
		return processMessageDelete(entry, batchProcessor)
	case queue.HandlerReactionAdd:
		return processReactionOperation(entry, batchProcessor)
	case queue.HandlerReactionDelete:
		return processReactionOperation(entry, batchProcessor)
	default:
		return fmt.Errorf("unknown handler: %s", entry.Handler)
	}
}

// Threads
func processThreadCreate(entry types.BatchEntry, batchProcessor *BatchProcessor) error {
	if entry.Author == "" {
		return fmt.Errorf("author required for thread creation")
	}

	// Require Model for thread creation (API/Compute layers ensure proper processing)
	if entry.Model == nil {
		return fmt.Errorf("model required for thread creation")
	}

	thread, ok := entry.Model.(*models.Thread)
	if !ok {
		return fmt.Errorf("invalid model type for thread creation")
	}

	// validate and set threadKey
	if keys.ValidateThreadKey(entry.TID) != nil && keys.ValidateThreadPrvKey(entry.TID) != nil {
		return fmt.Errorf("invalid thread key format: %s - expected t:<threadID>", entry.TID)
	}
	threadKey := entry.TID
	thread.ID = threadKey

	logger.Debug("thread_final_key_resolved", "tid", entry.TID, "final_key", threadKey)

	// finalize payload
	updatedPayload, err := json.Marshal(thread)
	if err != nil {
		return fmt.Errorf("marshal updated thread: %w", err)
	}

	// update thread indexes
	batchProcessor.Data.SetThreadMeta(threadKey, updatedPayload)
	batchProcessor.Index.InitThreadMessageIndexes(threadKey)
	batchProcessor.Index.UpdateUserOwnership(thread.Author, threadKey, true)
	batchProcessor.Index.UpdateThreadParticipants(threadKey, thread.Author, true)
	return nil
}

func processThreadDelete(entry types.BatchEntry, batchProcessor *BatchProcessor) error {
	if entry.TID == "" {
		return fmt.Errorf("thread ID required for thread deletion")
	}
	if entry.Author == "" {
		return fmt.Errorf("author required for thread deletion")
	}

	// block if anything else than provisional or final key
	if keys.ValidateThreadKey(entry.TID) != nil && keys.ValidateThreadPrvKey(entry.TID) != nil {
		return fmt.Errorf("invalid thread key format: %s - expected t:<threadID>", entry.TID)
	}

	threadKey := entry.TID
	author := entry.Author

	logger.Debug("thread_final_key_resolved_for_deletion", "tid", entry.TID, "final_key", threadKey)

	// Fetch existing thread data
	threadData, err := batchProcessor.Data.GetThreadMetaCopy(threadKey)
	if err != nil {
		logger.Debug("thread_not_found_for_delete", "thread_id", threadKey, "error", err)
		// Thread doesn't exist, just track deletion index
		batchProcessor.Index.UpdateUserOwnership(author, threadKey, false)
		batchProcessor.Index.UpdateSoftDeletedThreads(author, threadKey, true)
		return nil
	}

	// Parse existing thread and mark as deleted
	var thread models.Thread
	if err := json.Unmarshal(threadData, &thread); err != nil {
		return fmt.Errorf("unmarshal thread for delete: %w", err)
	}

	// Mark thread as deleted
	thread.Deleted = true
	thread.UpdatedTS = entry.TS

	// Serialize updated thread
	updatedPayload, err := json.Marshal(thread)
	if err != nil {
		return fmt.Errorf("marshal deleted thread: %w", err)
	}

	// Update thread data (soft delete)
	batchProcessor.Data.SetThreadMeta(threadKey, updatedPayload)
	batchProcessor.Index.UpdateUserOwnership(author, threadKey, false)
	batchProcessor.Index.UpdateSoftDeletedThreads(author, threadKey, true)
	return nil
}

func processThreadUpdate(entry types.BatchEntry, batchProcessor *BatchProcessor) error {
	// validate
	if entry.TID == "" {
		return fmt.Errorf("thread ID required for thread update")
	}
	if entry.Author == "" {
		return fmt.Errorf("author required for thread update")
	}

	if entry.Model == nil {
		return fmt.Errorf("model required for thread update")
	}

	if keys.ValidateThreadKey(entry.TID) != nil && keys.ValidateThreadPrvKey(entry.TID) != nil {
		return fmt.Errorf("invalid thread key format: %s - expected t:<threadID>", entry.TID)
	}

	// extract
	thread, ok := entry.Model.(*models.Thread)
	if !ok {
		return fmt.Errorf("invalid model type for thread update")
	}

	// fields
	threadKey := entry.TID
	thread.ID = threadKey
	thread.UpdatedTS = entry.TS

	logger.Debug("thread_final_key_resolved_for_update", "tid", entry.TID, "final_key", threadKey)

	// serialize
	updatedPayload, err := json.Marshal(thread)
	if err != nil {
		return fmt.Errorf("marshal updated thread: %w", err)
	}

	// sync
	batchProcessor.Data.SetThreadMeta(threadKey, updatedPayload)
	return nil
}

// Messages
func processMessageCreate(entry types.BatchEntry, batchProcessor *BatchProcessor) error {
	if entry.TID == "" {
		return fmt.Errorf("thread ID required for message create")
	}
	if entry.Model == nil {
		return fmt.Errorf("model required for message create")
	}

	msg, ok := entry.Model.(*models.Message)
	if !ok {
		return fmt.Errorf("invalid model type for message create")
	}

	// keys
	threadKey := entry.TID
	threadMessageKey := entry.MID
	if threadMessageKey == "" {
		return fmt.Errorf("message ID required for create")
	}

	// resolve
	resolvedID, err := batchProcessor.Index.ResolveMessageID(msg.ID, threadMessageKey)
	if err != nil {
		logger.Error("message_create_resolution_failed", "msg_id", msg.ID, "err", err)
		return fmt.Errorf("resolve message ID %s: %w", msg.ID, err)
	}
	threadMessageKey = resolvedID

	logger.Debug("message_final_key_resolved_for_create", "msg_id", msg.ID, "provisional_or_entry_key", entry.MID, "final_key", threadMessageKey)

	// sync
	if parts, err := keys.ParseMessageKey(threadMessageKey); err == nil {
		msg.ID = parts.MsgID
	} else if parts, err := keys.ParseMessageProvisionalKey(threadMessageKey); err == nil {
		msg.ID = parts.MsgID
	} else {
		msg.ID = threadMessageKey // fallback
	}
	msg.Thread = threadKey

	msg.TS = entry.TS

	updatedPayload, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal message: %w", err)
	}

	// Extract sequence from the resolved final key
	threadSequence := extractSequenceFromKey(threadMessageKey)

	// Update thread indexes (note: sequence already incremented in ResolveMessageID)
	batchProcessor.Index.UpdateThreadMessageIndexes(threadKey, msg.TS, entry.TS, false, "")

	// Store message data with thread-scoped sequence
	if err := batchProcessor.Data.SetMessageData(threadKey, threadMessageKey, updatedPayload, entry.TS, threadSequence); err != nil {
		return fmt.Errorf("setmessage data: %w", err)
	}
	return nil
}

func processMessageUpdate(entry types.BatchEntry, batchProcessor *BatchProcessor) error {
	logger.Debug("process_message_update", "thread", entry.TID, "msg", entry.MID)
	if entry.TID == "" {
		return fmt.Errorf("thread ID required for message update")
	}
	if entry.Model == nil {
		return fmt.Errorf("model required for message update")
	}

	threadKey := entry.TID

	msg, ok := entry.Model.(*models.Message)
	if !ok {
		return fmt.Errorf("invalid model type for message update")
	}

	// Resolve message ID using unified method (handles provisional/final automatically)
	// msg.ID should always be populated for message operations
	resolvedMessageID, err := batchProcessor.Index.ResolveMessageID(msg.ID, msg.ID)
	if err != nil {
		logger.Error("message_update_resolution_failed", "msg_id", msg.ID, "err", err)
		return fmt.Errorf("resolve message ID %s: %w", msg.ID, err)
	}

	logger.Debug("message_final_key_resolved_for_update", "msg_id", msg.ID, "original_msg_id", entry.MID, "final_key", resolvedMessageID)

	// sync
	if parts, err := keys.ParseMessageKey(resolvedMessageID); err == nil {
		msg.ID = parts.MsgID
	} else if parts, err := keys.ParseMessageProvisionalKey(resolvedMessageID); err == nil {
		msg.ID = parts.MsgID
	} else {
		msg.ID = resolvedMessageID // fallback
	}
	msg.Thread = threadKey
	if msg.TS == 0 {
		msg.TS = entry.TS
	}

	updatedPayload, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal message: %w", err)
	}

	// Extract sequence from the resolved final key
	threadSequence := extractSequenceFromKey(resolvedMessageID)

	if err := batchProcessor.Data.SetMessageData(threadKey, resolvedMessageID, updatedPayload, entry.TS, threadSequence); err != nil {
		return fmt.Errorf("set message data: %w", err)
	}
	versionKey := keys.GenVersionKey(resolvedMessageID, entry.TS, threadSequence)
	batchProcessor.Data.SetVersionKey(versionKey, updatedPayload)

	// Update indexes (no sequence increment for updates)
	threadComp, messageComp, err := keys.ExtractMessageComponents(threadKey, resolvedMessageID)
	if err != nil {
		return fmt.Errorf("extract message components: %w", err)
	}
	msgKey := keys.GenMessageKey(threadComp, messageComp, threadSequence)
	batchProcessor.Index.UpdateThreadMessageIndexes(threadKey, msg.TS, entry.TS, false, msgKey)

	return nil
}

func processMessageDelete(entry types.BatchEntry, batchProcessor *BatchProcessor) error {
	if entry.TID == "" {
		return fmt.Errorf("thread ID required for message deletion")
	}
	finalThreadID := entry.TID
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
	// Resolve message ID using unified method (handles provisional/final automatically)
	finalMessageID, err := batchProcessor.Index.ResolveMessageID(msg.ID, msg.ID)
	if err != nil {
		logger.Error("message_delete_resolution_failed", "msg_id", msg.ID, "handler", entry.Handler, "err", err)
		return fmt.Errorf("resolve message ID %s: %w", msg.ID, err)
	}

	logger.Debug("message_final_key_resolved_for_delete", "msg_id", msg.ID, "final_key", finalMessageID)

	// Fetch existing message data
	messageKey := keys.GenMessageKey(finalThreadID, finalMessageID, extractSequenceFromKey(finalMessageID))
	messageData, err := batchProcessor.Data.GetMessageDataCopy(messageKey)
	if err != nil {
		logger.Debug("message_not_found_for_delete", "message_id", finalMessageID, "error", err)
		return fmt.Errorf("message not found for delete: %s", finalMessageID)
	}

	// Parse existing message and mark as deleted
	var existingMessage models.Message
	if err := json.Unmarshal(messageData, &existingMessage); err != nil {
		return fmt.Errorf("unmarshal message for delete: %w", err)
	}

	// Mark message as deleted
	existingMessage.Deleted = true
	existingMessage.TS = entry.TS

	// Serialize updated message
	updatedPayload, err := json.Marshal(existingMessage)
	if err != nil {
		return fmt.Errorf("marshal deleted message: %w", err)
	}

	// Extract sequence from the resolved final key
	threadSequence := extractSequenceFromKey(finalMessageID)

	// Update message data (soft delete)
	if err := batchProcessor.Data.SetMessageData(finalThreadID, finalMessageID, updatedPayload, entry.TS, threadSequence); err != nil {
		return fmt.Errorf("set deleted message data: %w", err)
	}

	// Create version entry for the deletion
	versionKey := keys.GenVersionKey(finalMessageID, entry.TS, threadSequence)
	batchProcessor.Data.SetVersionKey(versionKey, updatedPayload)

	// Update indexes and track deletion
	batchProcessor.Index.UpdateThreadMessageIndexes(finalThreadID, existingMessage.TS, entry.TS, true, finalMessageID)
	batchProcessor.Index.UpdateSoftDeletedMessages(existingMessage.Author, finalMessageID, true)
	return nil
}

func processReactionOperation(entry types.BatchEntry, batchProcessor *BatchProcessor) error {
	logger.Debug("process_reaction", "thread", entry.TID, "msg", entry.MID, "handler", entry.Handler)
	if entry.TID == "" {
		return fmt.Errorf("thread ID required for reaction operation")
	}
	finalThreadID := entry.TID

	// Require Model for reaction operation (API/Compute layers ensure proper processing)
	if entry.Model == nil {
		return fmt.Errorf("model required for reaction operation")
	}

	msg, ok := entry.Model.(*models.Message)
	if !ok {
		return fmt.Errorf("invalid model type for reaction operation")
	}
	// Resolve message ID using unified method (handles provisional/final automatically)
	finalMessageID, err := batchProcessor.Index.ResolveMessageID(msg.ID, msg.ID)
	if err != nil {
		logger.Error("message_reaction_resolution_failed", "msg_id", msg.ID, "handler", entry.Handler, "err", err)
		return fmt.Errorf("resolve message ID %s: %w", msg.ID, err)
	}

	logger.Debug("message_final_key_resolved_for_reaction", "msg_id", msg.ID, "original_msg_id", entry.MID, "final_key", finalMessageID)

	msg.Thread = finalThreadID
	if parts, err := keys.ParseMessageKey(finalMessageID); err == nil {
		msg.ID = parts.MsgID
	} else if parts, err := keys.ParseMessageProvisionalKey(finalMessageID); err == nil {
		msg.ID = parts.MsgID
	} else {
		msg.ID = finalMessageID // fallback
	}
	updatedPayload, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal updated message: %w", err)
	}
	// Extract sequence from the resolved final key
	threadSequence := extractSequenceFromKey(finalMessageID)

	if err := batchProcessor.Data.SetMessageData(finalThreadID, finalMessageID, updatedPayload, entry.TS, threadSequence); err != nil {
		return fmt.Errorf("set message data: %w", err)
	}
	versionKey := keys.GenVersionKey(finalMessageID, entry.TS, threadSequence)
	batchProcessor.Data.SetVersionKey(versionKey, updatedPayload)
	batchProcessor.Index.UpdateThreadMessageIndexes(finalThreadID, msg.TS, entry.TS, false, finalMessageID)
	return nil
}
