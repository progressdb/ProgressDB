// Performs stateful computation & storage of mutative payloads
package apply

import (
	"encoding/json"
	"fmt"

	"progressdb/pkg/ingest/queue"
	"progressdb/pkg/ingest/types"
	"progressdb/pkg/models"
	"progressdb/pkg/store/db/index"
	"progressdb/pkg/store/keys"
	"progressdb/pkg/store/threads"
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
	if keys.ValidateThreadKey(threadKey) != nil && keys.ValidateThreadPrvKey(threadKey) != nil {
		return fmt.Errorf("invalid thread key format: %s - expected t:<threadID>", threadKey)
	}
	thread.Key = threadKey

	// store
	if err := batchProcessor.Data.SetThreadMeta(threadKey, thread); err != nil {
		return fmt.Errorf("set thread meta: %w", err)
	}

	// index
	batchProcessor.Index.InitThreadMessageIndexes(threadKey)
	batchProcessor.Index.UpdateUserOwnership(thread.Author, threadKey, true)
	batchProcessor.Index.UpdateThreadParticipants(threadKey, thread.Author, true)
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
	if keys.ValidateThreadKey(threadKey) != nil && keys.ValidateThreadPrvKey(threadKey) != nil {
		return fmt.Errorf("invalid thread key format: %s - expected t:<threadID>", threadKey)
	}

	// check access
	hasOwnership, err := index.DoesUserOwnThread(author, threadKey)
	if err != nil {
		return fmt.Errorf("failed to check thread ownership: %w", err)
	}

	hasParticipation, err := index.DoesThreadHaveUser(threadKey, author)
	if err != nil {
		return fmt.Errorf("failed to check thread participation: %w", err)
	}

	if !hasOwnership && !hasParticipation {
		return fmt.Errorf("access denied: user %s does not have access to thread %s", author, threadKey)
	}

	// store
	if err := index.MarkSoftDeleted(threadKey); err != nil {
		return fmt.Errorf("failed to mark thread as deleted: %w", err)
	}

	if hasOwnership {
		if err := index.UnmarkUserOwnsThread(author, threadKey); err != nil {
			return fmt.Errorf("failed to remove thread ownership: %w", err)
		}
	}

	if hasParticipation {
		if err := index.UnmarkThreadHasUser(threadKey, author); err != nil {
			return fmt.Errorf("failed to remove thread participation: %w", err)
		}
	}

	if err := index.DeleteThreadMessageIndexes(threadKey); err != nil {
		return fmt.Errorf("failed to clean up thread indexes: %w", err)
	}

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
	if keys.ValidateThreadKey(threadKey) != nil && keys.ValidateThreadPrvKey(threadKey) != nil {
		return fmt.Errorf("invalid thread key format: %s - expected t:<threadKey>", threadKey)
	}

	// check access
	hasOwnership, err := index.DoesUserOwnThread(author, threadKey)
	if err != nil {
		return fmt.Errorf("failed to check thread ownership: %w", err)
	}

	if !hasOwnership {
		return fmt.Errorf("access denied: only thread owner can update thread %s", threadKey)
	}

	// fetch existing
	existingData, err := batchProcessor.Data.GetThreadMetaCopy(threadKey)
	if err != nil {
		// Not in batch, fetch from DB
		data, err := threads.GetThread(threadKey)
		if err != nil {
			return fmt.Errorf("failed to get thread for update: %w", err)
		}
		existingData = []byte(data)
	}

	// parse existing
	var thread models.Thread
	if err := json.Unmarshal(existingData, &thread); err != nil {
		return fmt.Errorf("unmarshal existing thread: %w", err)
	}

	// apply updates
	if update.Title != "" {
		thread.Title = update.Title
	}
	if update.Slug != "" {
		thread.Slug = update.Slug
	}
	if update.UpdatedTS != 0 {
		thread.UpdatedTS = update.UpdatedTS
	}

	// store
	if err := batchProcessor.Data.SetThreadMeta(threadKey, &thread); err != nil {
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
	hasOwnership, err := index.DoesUserOwnThread(author, threadKey)
	if err != nil {
		return fmt.Errorf("failed to check thread ownership: %w", err)
	}

	hasParticipation, err := index.DoesThreadHaveUser(threadKey, author)
	if err != nil {
		return fmt.Errorf("failed to check thread participation: %w", err)
	}

	if !hasOwnership && !hasParticipation {
		return fmt.Errorf("access denied: user %s does not have access to thread %s", author, threadKey)
	}

	// resolve message key
	resolvedID, err := batchProcessor.Index.ResolveMessageKey(msg.Key, msg.Key)
	if err != nil {
		return fmt.Errorf("resolve message key %s: %w", msg.Key, err)
	}

	// sync message fields
	msg.Thread = threadKey
	msg.Author = author
	if msg.TS == 0 {
		msg.TS = entry.TS
	}

	// extract sequence
	threadSequence := extractSequenceFromKey(resolvedID)
	msg.Key = keys.GenMessageKey(threadKey, resolvedID, threadSequence)

	// index
	batchProcessor.Index.UpdateThreadMessageIndexes(threadKey, msg.TS, entry.TS, false, "")

	// store
	if err := batchProcessor.Data.SetMessageData(threadKey, resolvedID, msg, entry.TS, threadSequence); err != nil {
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

	// resolve message key
	resolvedMessageKey, err := batchProcessor.Index.ResolveMessageKey(messageKey, messageKey)
	if err != nil {
		return fmt.Errorf("resolve message key %s: %w", messageKey, err)
	}

	// fetch existing
	dbMessageKey := keys.GenMessageKey(threadKey, resolvedMessageKey, extractSequenceFromKey(resolvedMessageKey))
	existingData, err := batchProcessor.Data.GetMessageDataCopy(dbMessageKey)
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
	if update.TS != 0 {
		msg.TS = update.TS
	}

	// extract sequence
	threadSequence := extractSequenceFromKey(resolvedMessageKey)

	// store
	if err := batchProcessor.Data.SetMessageData(threadKey, resolvedMessageKey, &msg, entry.TS, threadSequence); err != nil {
		return fmt.Errorf("set message data: %w", err)
	}
	versionKey := keys.GenVersionKey(resolvedMessageKey, entry.TS, threadSequence)
	if err := batchProcessor.Data.SetVersionKey(versionKey, &msg); err != nil {
		return fmt.Errorf("set version key: %w", err)
	}

	// indexes
	threadComp, messageComp, err := keys.ExtractMessageComponents(threadKey, resolvedMessageKey)
	if err != nil {
		return fmt.Errorf("extract message components: %w", err)
	}
	msgKey := keys.GenMessageKey(threadComp, messageComp, threadSequence)
	batchProcessor.Index.UpdateThreadMessageIndexes(threadKey, msg.TS, entry.TS, false, msgKey)

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

	var msg models.Message
	if entry.Payload != nil {
		if m, ok := entry.Payload.(*models.Message); ok {
			msg = *m
		}
	}

	// resolve message key
	finalMessageKey, err := batchProcessor.Index.ResolveMessageKey(msg.Key, msg.Key)
	if err != nil {
		return fmt.Errorf("resolve message key %s: %w", msg.Key, err)
	}

	// fetch existing
	messageKey := keys.GenMessageKey(finalThreadKey, finalMessageKey, extractSequenceFromKey(finalMessageKey))
	messageData, err := batchProcessor.Data.GetMessageDataCopy(messageKey)
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
	existingMessage.TS = entry.TS

	// extract sequence
	threadSequence := extractSequenceFromKey(finalMessageKey)

	// store
	if err := batchProcessor.Data.SetMessageData(finalThreadKey, finalMessageKey, existingMessage, entry.TS, threadSequence); err != nil {
		return fmt.Errorf("set deleted message data: %w", err)
	}

	versionKey := keys.GenVersionKey(finalMessageKey, entry.TS, threadSequence)
	if err := batchProcessor.Data.SetVersionKey(versionKey, existingMessage); err != nil {
		return fmt.Errorf("set version key: %w", err)
	}

	// update indexes
	batchProcessor.Index.UpdateThreadMessageIndexes(finalThreadKey, existingMessage.TS, entry.TS, true, finalMessageKey)
	batchProcessor.Index.UpdateSoftDeletedMessages(existingMessage.Author, finalMessageKey, true)
	return nil
}
