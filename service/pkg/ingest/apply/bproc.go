// Performs stateful computation & storage of mutative payloads
package apply

import (
	"encoding/json"
	"fmt"

	"progressdb/pkg/ingest/types"
	"progressdb/pkg/models"
	"progressdb/pkg/state"
	"progressdb/pkg/state/logger"
	"progressdb/pkg/store/keys"
	"progressdb/pkg/store/slug"
)

func BProcOperation(entry types.BatchEntry, batchProcessor *BatchProcessor) error {
	switch entry.Handler {
	case types.HandlerThreadCreate:
		return BProcThreadCreate(entry, batchProcessor)
	case types.HandlerThreadUpdate:
		return BProcThreadUpdate(entry, batchProcessor)
	case types.HandlerThreadDelete:
		return BProcThreadDelete(entry, batchProcessor)
	case types.HandlerMessageCreate:
		return BProcMessageCreate(entry, batchProcessor)
	case types.HandlerMessageUpdate:
		return BProcMessageUpdate(entry, batchProcessor)
	case types.HandlerMessageDelete:
		return BProcMessageDelete(entry, batchProcessor)
	}

	// this is not going to happen
	// but if by magic it occurs
	// - crash the system (to prevent any blind ops)
	err := fmt.Errorf("BProcOperation: unsupported operation or handler")
	state.Crash("bproc_operation_unsupported_handler", err)
	return err
}

// Threads
func BProcThreadCreate(entry types.BatchEntry, batchProcessor *BatchProcessor) error {
	// extract
	author := extractAuthor(entry)
	if author == "" {
		return fmt.Errorf("author required for thread creation")
	}

	// validate
	if entry.Payload == nil {
		return fmt.Errorf("payload required for thread creation")
	}

	// parse
	thread, ok := entry.Payload.(*models.Thread)
	if !ok {
		return fmt.Errorf("invalid payload type for thread creation")
	}

	// resolve
	threadKey := ExtractTKey(entry.QueueOp)
	if _, err := keys.ParseKey(threadKey); err != nil {
		return fmt.Errorf("invalid thread key format: %s - expected t:<threadKey>", threadKey)
	}
	thread.Key = threadKey

	// Generate slug if not provided
	if thread.Slug == "" {
		thread.Slug = slug.GenerateSlug(thread.Title, threadKey)
	}

	// store
	if err := batchProcessor.Data.SetThreadData(threadKey, thread); err != nil {
		return fmt.Errorf("set thread meta: %w", err)
	}

	// index
	// thread <> message indexes are inited already
	batchProcessor.Index.SetUserOwnership(author, threadKey, 1)      // user, thread, 1
	batchProcessor.Index.SetThreadParticipants(author, threadKey, 1) // user, thread, 1
	return nil
}

func BProcThreadDelete(entry types.BatchEntry, batchProcessor *BatchProcessor) error {
	// extract
	author := extractAuthor(entry)
	if author == "" {
		return fmt.Errorf("author required for thread deletion")
	}

	// resolve
	threadKey := ExtractTKey(entry.QueueOp)
	if threadKey == "" {
		return fmt.Errorf("thread ID required for thread deletion")
	}

	// validate
	if _, err := keys.ParseKey(threadKey); err != nil {
		return fmt.Errorf("invalid thread key format: %s - expected t:<threadKey>", threadKey)
	}

	// check access
	hasOwnership, err := batchProcessor.Index.DoesUserOwnThread(author, threadKey)
	if err != nil {
		return fmt.Errorf("failed to check thread ownership: %w", err)
	}

	hasParticipation, err := batchProcessor.Index.DoesThreadHaveUser(threadKey, author)
	if err != nil {
		return fmt.Errorf("failed to check thread participation: %w", err)
	}

	if !hasOwnership && !hasParticipation {
		return fmt.Errorf("access denied: user %s does not have access to thread %s", author, threadKey)
	}

	// fetch existing
	existingData, err := batchProcessor.Data.GetThreadMetaCopy(threadKey)
	if err != nil {
		return fmt.Errorf("failed to get thread for delete: %w", err)
	}

	// parse existing
	var thread models.Thread
	if err := json.Unmarshal(existingData, &thread); err != nil {
		return fmt.Errorf("unmarshal existing thread: %w", err)
	}

	// apply delete
	thread.Deleted = true
	thread.UpdatedTS = entry.TS

	// store
	if err := batchProcessor.Data.SetThreadData(threadKey, &thread); err != nil {
		return fmt.Errorf("set thread meta: %w", err)
	}

	// index
	batchProcessor.Index.SetSoftDeletedThreads(author, threadKey, 1)

	return nil
}

func BProcThreadUpdate(entry types.BatchEntry, batchProcessor *BatchProcessor) error {
	// extract
	author := extractAuthor(entry)
	if author == "" {
		return fmt.Errorf("author required for thread update")
	}

	// validate
	if entry.Payload == nil {
		return fmt.Errorf("payload required for thread update")
	}

	// parse
	update, ok := entry.Payload.(*models.ThreadUpdatePartial)
	if !ok {
		return fmt.Errorf("invalid payload type for thread update")
	}

	// resolve
	threadKey := ExtractTKey(entry.QueueOp)
	if _, err := keys.ParseKey(threadKey); err != nil {
		return fmt.Errorf("invalid thread key format: %s - expected t:<threadKey>", threadKey)
	}

	// check access
	hasOwnership, err := batchProcessor.Index.DoesUserOwnThread(author, threadKey)
	if err != nil {
		return fmt.Errorf("failed to check thread ownership: %w", err)
	}

	if !hasOwnership {
		return fmt.Errorf("access denied: only thread owner can update thread %s", threadKey)
	}

	// fetch existing
	existingData, err := batchProcessor.Data.GetThreadMetaCopy(threadKey)
	if err != nil {
		return fmt.Errorf("failed to get thread for update: %w", err)
	}

	// parse existing
	var thread models.Thread
	if err := json.Unmarshal(existingData, &thread); err != nil {
		return fmt.Errorf("unmarshal existing thread: %w", err)
	}

	// apply updates
	if update.Title != "" {
		thread.Title = update.Title
		// Generate slug if title changed and slug is empty
		if thread.Slug == "" {
			thread.Slug = slug.GenerateSlug(thread.Title, threadKey)
		}
	}
	if update.Slug != "" {
		thread.Slug = update.Slug
	}
	if update.UpdatedTS != 0 {
		thread.UpdatedTS = update.UpdatedTS
	}

	// store
	if err := batchProcessor.Data.SetThreadData(threadKey, &thread); err != nil {
		return fmt.Errorf("set thread meta: %w", err)
	}

	return nil
}

// Messages
func BProcMessageCreate(entry types.BatchEntry, batchProcessor *BatchProcessor) error {
	// extract
	author := extractAuthor(entry)
	if author == "" {
		return fmt.Errorf("author required for message create")
	}

	// resolve
	threadKey := ExtractTKey(entry.QueueOp)
	if threadKey == "" {
		return fmt.Errorf("thread ID required for message create")
	}

	// validate
	if entry.Payload == nil {
		return fmt.Errorf("payload required for message create")
	}

	// parse
	msg, ok := entry.Payload.(*models.Message)
	if !ok {
		return fmt.Errorf("invalid payload type for message create")
	}

	// check access
	hasOwnership, err := batchProcessor.Index.DoesUserOwnThread(author, threadKey)
	if err != nil {
		return fmt.Errorf("failed to check thread ownership: %w", err)
	}

	hasParticipation, err := batchProcessor.Index.DoesThreadHaveUser(threadKey, author)
	if err != nil {
		return fmt.Errorf("failed to check thread participation: %w", err)
	}

	if !hasOwnership && !hasParticipation {
		return fmt.Errorf("access denied: user %s does not have access to thread %s", author, threadKey)
	}

	// resolve message key
	finalMessageKey, err := batchProcessor.Index.ResolveMessageKey(msg.Key)
	if err != nil {
		logger.Error("BProcMessageCreate: failed to resolve message key", "msg_key", msg.Key, "error", err)
		return fmt.Errorf("resolve message key %s: %w", msg.Key, err)
	}

	// sync message fields
	msg.Thread = threadKey
	msg.Author = author
	msg.CreatedTS = entry.TS
	msg.UpdatedTS = entry.TS
	msg.Key = finalMessageKey

	// index
	batchProcessor.Index.UpdateThreadMessageIndexes(threadKey, msg)

	// store
	if err := batchProcessor.Data.SetMessageData(finalMessageKey, msg, entry.TS); err != nil {
		return fmt.Errorf("set message data: %w", err)
	}
	return nil
}

func BProcMessageUpdate(entry types.BatchEntry, batchProcessor *BatchProcessor) error {
	// extract
	author := extractAuthor(entry)
	if author == "" {
		return fmt.Errorf("author required for message update")
	}

	// resolve
	threadKey := ExtractTKey(entry.QueueOp)
	if threadKey == "" {
		return fmt.Errorf("thread key required for message update")
	}
	messageKey := ExtractMKey(entry.QueueOp)

	// validate
	if entry.Payload == nil {
		return fmt.Errorf("payload required for message update")
	}

	// parse
	update, ok := entry.Payload.(*models.MessageUpdatePartial)
	if !ok {
		return fmt.Errorf("invalid payload type for message update")
	}

	// check access
	hasOwnership, err := batchProcessor.Index.DoesUserOwnThread(author, threadKey)
	if err != nil {
		return fmt.Errorf("failed to check thread ownership: %w", err)
	}

	hasParticipation, err := batchProcessor.Index.DoesThreadHaveUser(threadKey, author)
	if err != nil {
		return fmt.Errorf("failed to check thread participation: %w", err)
	}

	if !hasOwnership && !hasParticipation {
		return fmt.Errorf("access denied: user %s does not have access to thread %s", author, threadKey)
	}

	// resolve message key
	finalMessageKey, err := batchProcessor.Index.ResolveMessageKey(messageKey)
	if err != nil {
		return fmt.Errorf("resolve message key %s: %w", messageKey, err)
	}

	// fetch existing
	existingData, err := batchProcessor.Data.GetMessageDataCopy(finalMessageKey)
	if err != nil {
		return fmt.Errorf("failed to get message for update: %w", err)
	}

	// parse existing
	var msg models.Message
	if err := json.Unmarshal(existingData, &msg); err != nil {
		return fmt.Errorf("unmarshal existing message: %w", err)
	}

	// apply updates
	if update.Body != nil {
		msg.Body = update.Body
	}
	if update.UpdatedTS != 0 {
		msg.UpdatedTS = update.UpdatedTS
	}

	// store
	if err := batchProcessor.Data.SetMessageData(finalMessageKey, &msg, entry.TS); err != nil {
		return fmt.Errorf("set message data: %w", err)
	}

	// Parse the message key to extract the sequence part
	messageKeyParts, err := keys.ParseMessageKey(finalMessageKey)
	if err != nil {
		return fmt.Errorf("failed to parse message key %s: %w", finalMessageKey, err)
	}

	// Convert the sequence string to uint64
	messageSeq, err := keys.KeySequenceNumbered(messageKeyParts.Seq)
	if err != nil {
		return fmt.Errorf("failed to convert sequence %s to uint64: %w", messageKeyParts.Seq, err)
	}

	versionKey := keys.GenMessageVersionKey(finalMessageKey, entry.TS, messageSeq)
	if err := batchProcessor.Data.SetVersionKey(versionKey, &msg); err != nil {
		return fmt.Errorf("set version key: %w", err)
	}

	// indexes
	batchProcessor.Index.UpdateThreadMessageIndexes(threadKey, &msg)

	return nil
}

func BProcMessageDelete(entry types.BatchEntry, batchProcessor *BatchProcessor) error {
	// extract
	author := extractAuthor(entry)
	if author == "" {
		return fmt.Errorf("author required for message delete")
	}

	// resolve
	finalThreadKey := ExtractTKey(entry.QueueOp)
	if finalThreadKey == "" {
		return fmt.Errorf("thread key required for message deletion")
	}

	// check access
	hasOwnership, err := batchProcessor.Index.DoesUserOwnThread(author, finalThreadKey)
	if err != nil {
		return fmt.Errorf("failed to check thread ownership: %w", err)
	}

	hasParticipation, err := batchProcessor.Index.DoesThreadHaveUser(finalThreadKey, author)
	if err != nil {
		return fmt.Errorf("failed to check thread participation: %w", err)
	}

	if !hasOwnership && !hasParticipation {
		return fmt.Errorf("access denied: user %s does not have access to thread %s", author, finalThreadKey)
	}

	var msgKey string
	if entry.Payload != nil {
		switch p := entry.Payload.(type) {
		case *models.Message:
			msgKey = p.Key
		case *models.MessageDeletePartial:
			msgKey = p.Key
		}
	}

	// resolve message key
	finalMessageKey, err := batchProcessor.Index.ResolveMessageKey(msgKey)
	if err != nil {
		return fmt.Errorf("resolve message key %s: %w", msgKey, err)
	}

	// fetch existing
	messageData, err := batchProcessor.Data.GetMessageDataCopy(finalMessageKey)
	if err != nil {
		return fmt.Errorf("message not found for delete: %s", finalMessageKey)
	}

	// parse existing
	var existingMessage models.Message
	if err := json.Unmarshal(messageData, &existingMessage); err != nil {
		return fmt.Errorf("unmarshal message for delete: %w", err)
	}

	// mark deleted
	existingMessage.Deleted = true
	existingMessage.UpdatedTS = entry.TS

	// DEBUG: Log what we're storing
	logger.Debug("message_delete_storing", "key", finalMessageKey, "deleted", existingMessage.Deleted, "updatedTS", existingMessage.UpdatedTS)

	// store
	if err := batchProcessor.Data.SetMessageData(finalMessageKey, existingMessage, entry.TS); err != nil {
		return fmt.Errorf("set deleted message data: %w", err)
	}

	logger.Debug("parsing_message_sequence", "finalMessageKey", finalMessageKey)

	// Parse the message key to extract the sequence part
	messageKeyParts, err := keys.ParseMessageKey(finalMessageKey)
	if err != nil {
		logger.Error("failed_to_parse_message_key", "error", err, "finalMessageKey", finalMessageKey)
		return fmt.Errorf("failed to parse message key %s: %w", finalMessageKey, err)
	}

	// Convert the sequence string to uint64
	messageSeq, err := keys.KeySequenceNumbered(messageKeyParts.Seq)
	if err != nil {
		logger.Error("failed_to_convert_sequence", "error", err, "sequence", messageKeyParts.Seq)
		return fmt.Errorf("failed to convert sequence %s to uint64: %w", messageKeyParts.Seq, err)
	}
	logger.Debug("parsed_message_sequence", "messageSeq", messageSeq, "sequenceString", messageKeyParts.Seq)

	versionKey := keys.GenMessageVersionKey(finalMessageKey, entry.TS, messageSeq)
	logger.Debug("generated_version_key", "versionKey", versionKey)
	if err := batchProcessor.Data.SetVersionKey(versionKey, existingMessage); err != nil {
		logger.Error("failed_to_set_version_key", "error", err, "versionKey", versionKey)
		return fmt.Errorf("set version key: %w", err)
	}
	logger.Debug("set_version_key_complete", "versionKey", versionKey)

	// update indexes
	logger.Debug("updating_thread_indexes", "finalThreadKey", finalThreadKey, "messageDeleted", existingMessage.Deleted)
	batchProcessor.Index.UpdateThreadMessageIndexes(finalThreadKey, &existingMessage)
	logger.Debug("updated_thread_indexes_complete", "finalThreadKey", finalThreadKey)

	// DEBUG: Log before setting soft delete marker
	logger.Debug("about_to_set_soft_delete", "author", author, "finalMessageKey", finalMessageKey)

	batchProcessor.Index.SetSoftDeletedMessages(author, finalMessageKey, 1) // user, message, 1

	// DEBUG: Log after setting soft delete marker
	logger.Debug("completed_soft_delete", "author", author, "finalMessageKey", finalMessageKey)

	return nil
}
