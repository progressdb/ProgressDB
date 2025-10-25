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

func collectThreadIDsFromGroups(threadGroups map[string][]types.BatchEntry) []string {
	threadMap := make(map[string]bool)
	for threadID := range threadGroups {
		if threadID != "" {
			threadMap[threadID] = true
		}
	}
	threadIDs := make([]string, 0, len(threadMap))
	for threadID := range threadMap {
		threadIDs = append(threadIDs, threadID)
	}
	return threadIDs
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

	// put reqs into thread groups
	threadGroups := groupByThread(entries)
	logger.Debug("batch_grouped", "threads", len(threadGroups))
	for threadID, threadEntries := range threadGroups {
		logger.Debug("thread_group", "thread_id", threadID, "operations", len(threadEntries))
		for _, entry := range threadEntries {
			logger.Debug("thread_group_op", "thread_id", threadID, "handler", entry.Handler, "tid", entry.TID, "mid", entry.MID)
		}
	}

	// initialize thread sequences from database
	threadIDs := collectThreadIDsFromGroups(threadGroups)
	if len(threadIDs) > 0 {
		tr.Mark("init_thread_sequences")
		if err := batchIndexManager.InitializeThreadSequencesFromDB(threadIDs); err != nil {
			logger.Error("init_thread_sequences_failed", "err", err)
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
