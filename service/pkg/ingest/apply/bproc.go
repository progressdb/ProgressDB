// Performs stateful computation & storage of mutative payloads
package apply

import (
	"encoding/json"
	"fmt"

	"progressdb/pkg/ingest/queue"
	"progressdb/pkg/ingest/types"
	"progressdb/pkg/models"
	"progressdb/pkg/state"
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
	if keys.ValidateThreadKey(threadKey) != nil && keys.ValidateThreadPrvKey(threadKey) != nil {
		return fmt.Errorf("invalid thread key format: %s - expected t:<threadID>", threadKey)
	}
	thread.Key = threadKey

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
	if keys.ValidateThreadKey(threadKey) != nil && keys.ValidateThreadPrvKey(threadKey) != nil {
		return fmt.Errorf("invalid thread key format: %s - expected t:<threadID>", threadKey)
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
	if keys.ValidateThreadKey(threadKey) != nil && keys.ValidateThreadPrvKey(threadKey) != nil {
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
		return fmt.Errorf("resolve message key %s: %w", msg.Key, err)
	}

	// sync message fields
	msg.Thread = threadKey
	msg.Author = author
	if msg.TS == 0 {
		msg.TS = entry.TS
	}

	// extract sequence
	messageSeq, err := extractMessageSequence(finalMessageKey)
	if err != nil {
		return fmt.Errorf("invalid resolved message key: %s: %w", finalMessageKey, err)
	}
	msg.Key = keys.GenMessageKey(threadKey, finalMessageKey, messageSeq)

	// index
	batchProcessor.Index.UpdateThreadMessageIndexes(threadKey, msg)

	// store
	if err := batchProcessor.Data.SetMessageData(threadKey, finalMessageKey, msg, entry.TS, messageSeq); err != nil {
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

	// extract sequence before use
	messageSeq, err := extractMessageSequence(finalMessageKey)
	if err != nil {
		return fmt.Errorf("invalid resolved message key: %s: %w", finalMessageKey, err)
	}

	// fetch existing
	dbMessageKey := keys.GenMessageKey(threadKey, finalMessageKey, messageSeq)
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

	// store
	if err := batchProcessor.Data.SetMessageData(threadKey, finalMessageKey, &msg, entry.TS, messageSeq); err != nil {
		return fmt.Errorf("set message data: %w", err)
	}
	versionKey := keys.GenVersionKey(finalMessageKey, entry.TS, messageSeq)
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

	var msg models.Message
	if entry.Payload != nil {
		if m, ok := entry.Payload.(*models.Message); ok {
			msg = *m
		}
	}

	// resolve message key
	finalMessageKey, err := batchProcessor.Index.ResolveMessageKey(msg.Key)
	if err != nil {
		return fmt.Errorf("resolve message key %s: %w", msg.Key, err)
	}

	// extract sequence before use
	messageSeq, err := extractMessageSequence(finalMessageKey)
	if err != nil {
		return fmt.Errorf("invalid resolved message key: %s: %w", finalMessageKey, err)
	}

	// fetch existing
	messageKey := keys.GenMessageKey(finalThreadKey, finalMessageKey, messageSeq)
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

	// store
	if err := batchProcessor.Data.SetMessageData(finalThreadKey, finalMessageKey, existingMessage, entry.TS, messageSeq); err != nil {
		return fmt.Errorf("set deleted message data: %w", err)
	}

	versionKey := keys.GenVersionKey(finalMessageKey, entry.TS, messageSeq)
	if err := batchProcessor.Data.SetVersionKey(versionKey, existingMessage); err != nil {
		return fmt.Errorf("set version key: %w", err)
	}

	// update indexes
	batchProcessor.Index.UpdateThreadMessageIndexes(finalThreadKey, &existingMessage)
	batchProcessor.Index.SetSoftDeletedMessages(author, finalMessageKey, 1) // user, message, 1
	return nil
}
