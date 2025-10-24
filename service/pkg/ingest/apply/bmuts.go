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

func processOperation(entry types.BatchEntry, batchIndexManager *BatchIndexManager) error {
	switch entry.Handler {
	case queue.HandlerThreadCreate:
		return processThreadCreate(entry, batchIndexManager)
	case queue.HandlerThreadUpdate:
		return processThreadUpdate(entry, batchIndexManager)
	case queue.HandlerThreadDelete:
		return processThreadDelete(entry, batchIndexManager)
	case queue.HandlerMessageCreate:
		return processMessageCreate(entry, batchIndexManager)
	case queue.HandlerMessageUpdate:
		return processMessageUpdate(entry, batchIndexManager)
	case queue.HandlerMessageDelete:
		return processMessageDelete(entry, batchIndexManager)
	case queue.HandlerReactionAdd, queue.HandlerReactionDelete:
		return processReactionOperation(entry, batchIndexManager)
	default:
		return fmt.Errorf("unknown operation handler: %s", entry.Handler)
	}
}

// Threads
func processThreadCreate(entry types.BatchEntry, batchIndexManager *BatchIndexManager) error {
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

	// finalize payload
	updatedPayload, err := json.Marshal(thread)
	if err != nil {
		return fmt.Errorf("marshal updated thread: %w", err)
	}

	// update thread indexes
	batchIndexManager.SetThreadMeta(threadKey, updatedPayload)
	batchIndexManager.InitThreadMessageIndexes(threadKey)
	batchIndexManager.AddThreadToUser(thread.Author, threadKey)
	batchIndexManager.AddParticipantToThread(threadKey, thread.Author)
	return nil
}

func processThreadDelete(entry types.BatchEntry, batchIndexManager *BatchIndexManager) error {
	if entry.TID == "" {
		return fmt.Errorf("thread ID required for thread deletion")
	}
	if entry.Author == "" {
		return fmt.Errorf("author required for thread deletion")
	}
	batchIndexManager.mu.Lock()

	// block if anything else than provisional or final key
	if keys.ValidateThreadKey(entry.TID) != nil && keys.ValidateThreadPrvKey(entry.TID) != nil {
		return fmt.Errorf("invalid thread key format: %s - expected t:<threadID>", entry.TID)
	}

	threadKey := entry.TID
	author := entry.Author

	// sync indexes
	batchIndexManager.mu.Unlock()
	batchIndexManager.DeleteThreadMeta(threadKey)
	batchIndexManager.DeleteThreadMessageIndexes(threadKey)
	batchIndexManager.RemoveThreadFromUser(author, threadKey)
	batchIndexManager.AddDeletedThreadToUser(author, threadKey)
	return nil
}

func processThreadUpdate(entry types.BatchEntry, batchIndexManager *BatchIndexManager) error {
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

	// serialize
	updatedPayload, err := json.Marshal(thread)
	if err != nil {
		return fmt.Errorf("marshal updated thread: %w", err)
	}

	// sync
	batchIndexManager.SetThreadMeta(threadKey, updatedPayload)
	return nil
}

// Messages
func processMessageCreate(entry types.BatchEntry, batchIndexManager *BatchIndexManager) error {
	if entry.TID == "" {
		return fmt.Errorf("thread ID required for message create")
	}
	if entry.Model == nil {
		return fmt.Errorf("model required for message create")
	}

	threadKey := entry.TID

	msg, ok := entry.Model.(*models.Message)
	if !ok {
		return fmt.Errorf("invalid model type for message create")
	}

	// assume final key
	threadMessageKey := entry.MID
	if threadMessageKey == "" {
		return fmt.Errorf("message ID required for create")
	}

	// handle if provisional key
	if msg.ID != "" && batchIndexManager.messageSequencer.IsProvisionalMessageKey(msg.ID) && msg.ID != threadMessageKey {
		batchIndexManager.mu.Lock()
		batchIndexManager.messageSequencer.MapProvisionalToFinalMessageKey(msg.ID, threadMessageKey)
		batchIndexManager.mu.Unlock()
		logger.Debug("mapped_provisional_message", "provisional", msg.ID, "final", threadMessageKey)
	}

	// sync
	msg.ID = threadMessageKey
	msg.Thread = threadKey
	if msg.TS == 0 {
		msg.TS = entry.TS
	}

	updatedPayload, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal message: %w", err)
	}

	// Update thread message indexes first to get thread-scoped sequence
	batchIndexManager.UpdateThreadMessageIndexes(threadKey, msg.TS, entry.TS, false, "")

	// Get the thread-scoped sequence number
	batchIndexManager.mu.Lock()
	threadSequence := batchIndexManager.threadMessages[threadKey].End
	batchIndexManager.mu.Unlock()

	// Store message data with thread-scoped sequence
	if err := batchIndexManager.SetMessageData(threadKey, threadMessageKey, updatedPayload, entry.TS, threadSequence); err != nil {
		return fmt.Errorf("set message data: %w", err)
	}
	return nil
}

func processMessageUpdate(entry types.BatchEntry, batchIndexManager *BatchIndexManager) error {
	logger.Debug("process_message_update", "thread", entry.TID, "msg", entry.MID)
	if entry.TID == "" {
		return fmt.Errorf("thread ID required for message update")
	}
	if entry.Model == nil {
		return fmt.Errorf("model required for message update")
	}

	finalThreadID := entry.TID

	msg, ok := entry.Model.(*models.Message)
	if !ok {
		return fmt.Errorf("invalid model type for message update")
	}

	finalMessageID := entry.MID
	if finalMessageID == "" {
		return fmt.Errorf("message ID required for update")
	}

	// Resolve provisional to final message ID if needed
	batchIndexManager.mu.Lock()
	resolvedMessageID, err := batchIndexManager.messageSequencer.GetFinalMessageKey(finalMessageID)
	batchIndexManager.mu.Unlock()
	if err != nil {
		logger.Error("message_resolution_failed", "provisional_mid", finalMessageID, "err", err)
		return fmt.Errorf("resolve message ID %s: %w", finalMessageID, err)
	}

	msg.ID = resolvedMessageID
	msg.Thread = finalThreadID
	if msg.TS == 0 {
		msg.TS = entry.TS
	}

	updatedPayload, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal message: %w", err)
	}

	// For updates, get existing thread-scoped sequence (don't increment)
	batchIndexManager.mu.Lock()
	var threadSequence uint64
	if threadIdx := batchIndexManager.threadMessages[finalThreadID]; threadIdx != nil {
		threadSequence = threadIdx.End
	} else {
		threadSequence = 0
	}
	batchIndexManager.mu.Unlock()

	if err := batchIndexManager.SetMessageData(finalThreadID, resolvedMessageID, updatedPayload, entry.TS, threadSequence); err != nil {
		return fmt.Errorf("set message data: %w", err)
	}
	if err := batchIndexManager.AddMessageVersion(resolvedMessageID, updatedPayload, entry.TS, threadSequence); err != nil {
		return fmt.Errorf("add message version: %w", err)
	}

	// Update indexes (no sequence increment for updates)
	threadComp, messageComp, err := keys.ExtractMessageComponents(finalThreadID, resolvedMessageID)
	if err != nil {
		return fmt.Errorf("extract message components: %w", err)
	}
	msgKey := keys.GenMessageKey(threadComp, messageComp, threadSequence)
	batchIndexManager.UpdateThreadMessageIndexes(finalThreadID, msg.TS, entry.TS, false, msgKey)

	return nil
}

func processMessageDelete(entry types.BatchEntry, batchIndexManager *BatchIndexManager) error {
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
	batchIndexManager.mu.Lock()
	finalMessageID, err := batchIndexManager.messageSequencer.GetFinalMessageKey(msg.ID)
	batchIndexManager.mu.Unlock()
	if err != nil {
		logger.Error("message_resolution_failed", "provisional_mid", msg.ID, "handler", entry.Handler, "err", err)
		return fmt.Errorf("resolve message ID %s: %w", msg.ID, err)
	}
	// Extract components from provisional keys before generating final key
	threadComp, messageComp, err := keys.ExtractMessageComponents(finalThreadID, finalMessageID)
	if err != nil {
		return fmt.Errorf("extract message components: %w", err)
	}
	msgKey := keys.GenMessageKey(threadComp, messageComp, uint64(entry.Enq))
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
	finalThreadID := entry.TID

	// Require Model for reaction operation (API/Compute layers ensure proper processing)
	if entry.Model == nil {
		return fmt.Errorf("model required for reaction operation")
	}

	msg, ok := entry.Model.(*models.Message)
	if !ok {
		return fmt.Errorf("invalid model type for reaction operation")
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
	// Extract components from provisional keys before generating final key
	threadComp, messageComp, err := keys.ExtractMessageComponents(finalThreadID, finalMessageID)
	if err != nil {
		return fmt.Errorf("extract message components: %w", err)
	}
	msgKey := keys.GenMessageKey(threadComp, messageComp, uint64(entry.Enq))
	batchIndexManager.UpdateThreadMessageIndexes(finalThreadID, msg.TS, entry.TS, false, msgKey)
	return nil
}
