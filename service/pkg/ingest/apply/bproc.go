// Performs stateful computation & storage of mutative payloads
package apply

import (
	"encoding/json"
	"fmt"

	"progressdb/pkg/ingest/queue"
	"progressdb/pkg/ingest/types"
	"progressdb/pkg/logger"
	"progressdb/pkg/models"
	"progressdb/pkg/store/db/index"
	"progressdb/pkg/store/keys"
)

func BProcOperation(entry types.BatchEntry, batchProcessor *BatchProcessor) error {
	switch entry.Handler {
	case queue.HandlerThreadCreate:
		return BProcThreadCreate(entry, batchProcessor)
	case queue.HandlerThreadUpdate:
		return BProcThreadUpdate(entry, batchProcessor)
	case queue.HandlerThreadDelete:
		return BProcThreadDelete(entry, batchProcessor)
	case queue.HandlerMessageCreate:
		return BProcMessageCreate(entry, batchProcessor)
	case queue.HandlerMessageUpdate:
		return BProcMessageUpdate(entry, batchProcessor)
	case queue.HandlerMessageDelete:
		return BProcMessageDelete(entry, batchProcessor)

	default:
		return fmt.Errorf("unknown handler: %s", entry.Handler)
	}
}

// Threads
func BProcThreadCreate(entry types.BatchEntry, batchProcessor *BatchProcessor) error {
	author := extractAuthor(entry)
	if author == "" {
		return fmt.Errorf("author required for thread creation")
	}

	// Require Payload for thread creation
	if entry.Payload == nil {
		return fmt.Errorf("payload required for thread creation")
	}

	thread, ok := entry.Payload.(*models.Thread)
	if !ok {
		return fmt.Errorf("invalid payload type for thread creation")
	}

	// validate and set threadKey
	threadKey := extractTID(entry)
	if keys.ValidateThreadKey(threadKey) != nil && keys.ValidateThreadPrvKey(threadKey) != nil {
		return fmt.Errorf("invalid thread key format: %s - expected t:<threadID>", threadKey)
	}
	thread.ID = threadKey

	logger.Debug("thread_final_key_resolved", "tid", threadKey, "final_key", threadKey)

	// store
	batchProcessor.Data.SetThreadMeta(threadKey, thread)
	batchProcessor.Index.InitThreadMessageIndexes(threadKey)
	batchProcessor.Index.UpdateUserOwnership(thread.Author, threadKey, true)
	batchProcessor.Index.UpdateThreadParticipants(threadKey, thread.Author, true)
	return nil
}

func BProcThreadDelete(entry types.BatchEntry, batchProcessor *BatchProcessor) error {
	if entry.TID == "" {
		return fmt.Errorf("thread ID required for thread deletion")
	}
	author := extractAuthor(entry)
	if author == "" {
		return fmt.Errorf("author required for thread deletion")
	}

	// block if anything else than provisional or final key
	if keys.ValidateThreadKey(entry.TID) != nil && keys.ValidateThreadPrvKey(entry.TID) != nil {
		return fmt.Errorf("invalid thread key format: %s - expected t:<threadID>", entry.TID)
	}

	threadKey := entry.TID
	logger.Debug("thread_final_key_resolved_for_deletion", "tid", entry.TID, "final_key", threadKey)

	// Fetch existing thread data

	// Validate user-thread relationships (ownership or participation)
	// Skip validation if this is a system operation or if thread creation is in the same batch
	hasOwnership, err := index.DoesUserOwnThread(author, threadKey)
	if err != nil {
		logger.Error("thread_ownership_check_failed", "author", author, "thread", threadKey, "error", err)
		return fmt.Errorf("failed to check thread ownership: %w", err)
	}

	hasParticipation, err := index.DoesThreadHaveUser(threadKey, author)
	if err != nil {
		logger.Error("thread_participation_check_failed", "author", author, "thread", threadKey, "error", err)
		return fmt.Errorf("failed to check thread participation: %w", err)
	}

	if !hasOwnership && !hasParticipation {
		return fmt.Errorf("access denied: user %s does not have access to thread %s", author, threadKey)
	}

	logger.Debug("thread_access_validated", "author", author, "thread", threadKey, "ownership", hasOwnership, "participation", hasParticipation)

	// Mark thread as soft deleted
	if err := index.MarkSoftDeleted(threadKey); err != nil {
		logger.Error("thread_soft_delete_failed", "thread", threadKey, "error", err)
		return fmt.Errorf("failed to mark thread as deleted: %w", err)
	}

	// Remove from user ownership index
	if hasOwnership {
		if err := index.UnmarkUserOwnsThread(author, threadKey); err != nil {
			logger.Error("thread_ownership_removal_failed", "author", author, "thread", threadKey, "error", err)
			return fmt.Errorf("failed to remove thread ownership: %w", err)
		}
	}

	// Remove from participation index
	if hasParticipation {
		if err := index.UnmarkThreadHasUser(threadKey, author); err != nil {
			logger.Error("thread_participation_removal_failed", "author", author, "thread", threadKey, "error", err)
			return fmt.Errorf("failed to remove thread participation: %w", err)
		}
	}

	// Clean up thread message indexes
	if err := index.DeleteThreadMessageIndexes(threadKey); err != nil {
		logger.Error("thread_indexes_cleanup_failed", "thread", threadKey, "error", err)
		return fmt.Errorf("failed to clean up thread indexes: %w", err)
	}

	logger.Debug("thread_deleted_successfully", "thread", threadKey)
	return nil
}

func BProcThreadUpdate(entry types.BatchEntry, batchProcessor *BatchProcessor) error {
	author := extractAuthor(entry)
	if author == "" {
		return fmt.Errorf("author required for thread update")
	}

	if entry.Payload == nil {
		return fmt.Errorf("payload required for thread update")
	}

	update, ok := entry.Payload.(*models.ThreadUpdatePartial)
	if !ok {
		return fmt.Errorf("invalid payload type for thread update")
	}

	// validate thread key
	threadKey := extractTID(entry)
	if keys.ValidateThreadKey(threadKey) != nil && keys.ValidateThreadPrvKey(threadKey) != nil {
		return fmt.Errorf("invalid thread key format: %s - expected t:<threadID>", threadKey)
	}

	// Validate user-thread relationships (must be owner for updates)
	hasOwnership, err := index.DoesUserOwnThread(author, threadKey)
	if err != nil {
		logger.Error("thread_ownership_check_failed", "author", author, "thread", threadKey, "error", err)
		return fmt.Errorf("failed to check thread ownership: %w", err)
	}

	if !hasOwnership {
		return fmt.Errorf("access denied: only thread owner can update thread %s", threadKey)
	}

	logger.Debug("thread_update_access_validated", "author", author, "thread", threadKey)

	// For now, thread updates are not fully implemented since we can't retrieve existing thread data
	// This would require adding a GetThreadMeta method to the DataManager
	// For the partial update, we'd need to:
	// 1. Get existing thread data
	// 2. Apply the partial updates
	// 3. Store the updated thread data

	logger.Warn("thread_update_not_fully_implemented", "thread", threadKey, "update", update)
	return fmt.Errorf("thread update not fully implemented")
}

// Messages
func BProcMessageCreate(entry types.BatchEntry, batchProcessor *BatchProcessor) error {
	if entry.TID == "" {
		return fmt.Errorf("thread ID required for message create")
	}
	if entry.Payload == nil {
		return fmt.Errorf("payload required for message create")
	}

	msg, ok := entry.Payload.(*models.Message)
	if !ok {
		return fmt.Errorf("invalid payload type for message create")
	}

	// keys
	threadKey := entry.TID
	author := extractAuthor(entry)
	if author == "" {
		return fmt.Errorf("author required for message create")
	}

	// Validate user-thread relationships (ownership or participation)
	hasOwnership, err := index.DoesUserOwnThread(author, threadKey)
	if err != nil {
		logger.Error("thread_ownership_check_failed", "author", author, "thread", threadKey, "error", err)
		return fmt.Errorf("failed to check thread ownership: %w", err)
	}

	hasParticipation, err := index.DoesThreadHaveUser(threadKey, author)
	if err != nil {
		logger.Error("thread_participation_check_failed", "author", author, "thread", threadKey, "error", err)
		return fmt.Errorf("failed to check thread participation: %w", err)
	}

	if !hasOwnership && !hasParticipation {
		return fmt.Errorf("access denied: user %s does not have access to thread %s", author, threadKey)
	}

	logger.Debug("thread_access_validated", "author", author, "thread", threadKey, "ownership", hasOwnership, "participation", hasParticipation)

	// resolve message ID
	resolvedID, err := batchProcessor.Index.ResolveMessageID(msg.ID, msg.ID)
	if err != nil {
		logger.Error("message_create_resolution_failed", "msg_id", msg.ID, "err", err)
		return fmt.Errorf("resolve message ID %s: %w", msg.ID, err)
	}

	logger.Debug("message_final_key_resolved_for_create", "msg_id", msg.ID, "final_key", resolvedID)

	// sync message fields
	if parts, err := keys.ParseMessageKey(resolvedID); err == nil {
		msg.ID = parts.MsgID
	} else if parts, err := keys.ParseMessageProvisionalKey(resolvedID); err == nil {
		msg.ID = parts.MsgID
	} else {
		msg.ID = resolvedID // fallback
	}
	msg.Thread = threadKey
	msg.Author = author
	if msg.TS == 0 {
		msg.TS = entry.TS
	}

	// Extract sequence from the resolved final key
	threadSequence := extractSequenceFromKey(resolvedID)

	// Update thread indexes (sequence already incremented in ResolveMessageID)
	batchProcessor.Index.UpdateThreadMessageIndexes(threadKey, msg.TS, entry.TS, false, "")

	// Store message data with thread-scoped sequence
	if err := batchProcessor.Data.SetMessageData(threadKey, resolvedID, msg, entry.TS, threadSequence); err != nil {
		return fmt.Errorf("set message data: %w", err)
	}
	return nil
}

func BProcMessageUpdate(entry types.BatchEntry, batchProcessor *BatchProcessor) error {
	logger.Debug("process_message_update", "thread", entry.TID, "msg", entry.MID)
	if entry.TID == "" {
		return fmt.Errorf("thread ID required for message update")
	}
	if entry.Payload == nil {
		return fmt.Errorf("payload required for message update")
	}

	update, ok := entry.Payload.(*models.MessageUpdatePartial)
	if !ok {
		return fmt.Errorf("invalid payload type for message update")
	}

	// Resolve message ID
	resolvedMessageID, err := batchProcessor.Index.ResolveMessageID(entry.MID, entry.MID)
	if err != nil {
		logger.Error("message_update_resolution_failed", "msg_id", entry.MID, "err", err)
		return fmt.Errorf("resolve message ID %s: %w", entry.MID, err)
	}

	logger.Debug("message_final_key_resolved_for_update", "msg_id", entry.MID, "final_key", resolvedMessageID)

	// Fetch existing message
	messageKey := keys.GenMessageKey(entry.TID, resolvedMessageID, extractSequenceFromKey(resolvedMessageID))
	existingData, err := batchProcessor.Data.GetMessageDataCopy(messageKey)
	if err != nil {
		return fmt.Errorf("failed to get message for update: %w", err)
	}

	var msg models.Message
	if err := json.Unmarshal(existingData, &msg); err != nil {
		return fmt.Errorf("unmarshal existing message: %w", err)
	}

	// Apply updates
	if update.Body != nil {
		msg.Body = update.Body
	}
	if update.TS != nil {
		msg.TS = *update.TS
	}

	// Extract sequence from the resolved final key
	threadSequence := extractSequenceFromKey(resolvedMessageID)

	if err := batchProcessor.Data.SetMessageData(entry.TID, resolvedMessageID, &msg, entry.TS, threadSequence); err != nil {
		return fmt.Errorf("set message data: %w", err)
	}
	versionKey := keys.GenVersionKey(resolvedMessageID, entry.TS, threadSequence)
	batchProcessor.Data.SetVersionKey(versionKey, &msg)

	// Update indexes (no sequence increment for updates)
	threadComp, messageComp, err := keys.ExtractMessageComponents(entry.TID, resolvedMessageID)
	if err != nil {
		return fmt.Errorf("extract message components: %w", err)
	}
	msgKey := keys.GenMessageKey(threadComp, messageComp, threadSequence)
	batchProcessor.Index.UpdateThreadMessageIndexes(entry.TID, msg.TS, entry.TS, false, msgKey)

	return nil
}

func BProcMessageDelete(entry types.BatchEntry, batchProcessor *BatchProcessor) error {
	if entry.TID == "" {
		return fmt.Errorf("thread ID required for message deletion")
	}
	finalThreadID := entry.TID
	var msg models.Message
	if entry.Payload != nil {
		if m, ok := entry.Payload.(*models.Message); ok {
			msg = *m
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

	// Extract sequence from the resolved final key
	threadSequence := extractSequenceFromKey(finalMessageID)

	// Update message data (soft delete)
	if err := batchProcessor.Data.SetMessageData(finalThreadID, finalMessageID, existingMessage, entry.TS, threadSequence); err != nil {
		return fmt.Errorf("set deleted message data: %w", err)
	}

	// Create version entry for the deletion
	versionKey := keys.GenVersionKey(finalMessageID, entry.TS, threadSequence)
	batchProcessor.Data.SetVersionKey(versionKey, existingMessage)

	// Update indexes and track deletion
	batchProcessor.Index.UpdateThreadMessageIndexes(finalThreadID, existingMessage.TS, entry.TS, true, finalMessageID)
	batchProcessor.Index.UpdateSoftDeletedMessages(existingMessage.Author, finalMessageID, true)
	return nil
}
