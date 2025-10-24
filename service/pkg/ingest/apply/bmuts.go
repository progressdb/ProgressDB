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
	var thread models.Thread
	if err := json.Unmarshal(entry.Payload, &thread); err != nil {
		return fmt.Errorf("unmarshal thread: %w", err)
	}

	// no changes - set threadKey
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
	batchIndexManager.mu.Lock()

	// block if anything else than provisional or finak key
	if keys.ValidateThreadKey(entry.TID) != nil && keys.ValidateThreadPrvKey(entry.TID) != nil {
		return fmt.Errorf("invalid thread key format: %s - expected t:<threadID>", entry.TID)
	}


	threadKey := entry.TID

	batchIndexManager.mu.Unlock()
	batchIndexManager.DeleteThreadMeta(threadKey)
	batchIndexManager.DeleteThreadMessageIndexes(threadKey)
	batchIndexManager.RemoveThreadFromUser(thread.Author, threadKey)
	batchIndexManager.AddDeletedThreadToUser(thread.Author, threadKey)
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

	// Use entry.MID as primary source for final message ID
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

	// If message has a provisional ID, map it to the final ID
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
	// Extract components from provisional keys before generating final key
	threadComp, messageComp, err := keys.ExtractMessageComponents(finalThreadID, finalMessageID)
	if err != nil {
		return fmt.Errorf("extract message components: %w", err)
	}
	msgKey := keys.GenMessageKey(threadComp, messageComp, uint64(entry.Enq))
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

	// Use entry.MID as primary source for final message ID
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

	// If message has a provisional ID, map it to the final ID
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
	// Extract components from provisional keys before generating final key
	threadComp, messageComp, err := keys.ExtractMessageComponents(finalThreadID, finalMessageID)
	if err != nil {
		return fmt.Errorf("extract message components: %w", err)
	}
	msgKey := keys.GenMessageKey(threadComp, messageComp, uint64(entry.Enq))
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
	// Extract components from provisional keys before generating final key
	threadComp, messageComp, err := keys.ExtractMessageComponents(finalThreadID, finalMessageID)
	if err != nil {
		return fmt.Errorf("extract message components: %w", err)
	}
	msgKey := keys.GenMessageKey(threadComp, messageComp, uint64(entry.Enq))
	batchIndexManager.UpdateThreadMessageIndexes(finalThreadID, msg.TS, entry.TS, false, msgKey)
	return nil
}
