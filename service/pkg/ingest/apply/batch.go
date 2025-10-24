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

	"progressdb/pkg/telemetry"
)

func groupByThread(entries []types.BatchEntry) map[string][]types.BatchEntry {
	threadGroups := make(map[string][]types.BatchEntry)
	for _, entry := range entries {
		threadID := entry.TID // primary thread identifier

		// If this entry is a thread creation without TID, try to extract it
		if threadID == "" && entry.Handler == queue.HandlerThreadCreate {
			// Try extracting thread ID from JSON payload
			if len(entry.Payload) > 0 {
				var thread struct {
					ID string `json:"id"`
				}
				if err := json.Unmarshal(entry.Payload, &thread); err == nil && thread.ID != "" {
					threadID = thread.ID
				}
			}
			// Fallback: Try extracting thread ID from model object
			if threadID == "" && entry.Model != nil {
				if thread, ok := entry.Model.(*models.Thread); ok && thread.ID != "" {
					threadID = thread.ID
				}
			}
		}
		// Group entry under its threadID (may still be "")
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
