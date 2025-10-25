package apply

import (
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
		threadID := extractTID(entry) // primary thread identifier

		// Group entry under its threadID (may still be "")
		threadGroups[threadID] = append(threadGroups[threadID], entry)
	}
	return threadGroups
}

func getOperationPriority(handler queue.HandlerID) int {
	switch handler {
	case queue.HandlerThreadCreate, queue.HandlerThreadUpdate:
		return 1 // Thread operations first
	case queue.HandlerMessageCreate, queue.HandlerMessageUpdate:
		return 2 // Message operations second
	case queue.HandlerThreadDelete, queue.HandlerMessageDelete:
		return 3 // Delete operations last
	default:
		return 2
	}
}

func extractTS(entry types.BatchEntry) int64 {
	switch entry.Handler {
	case queue.HandlerThreadCreate:
		if th, ok := entry.Payload.(*models.Thread); ok {
			return th.CreatedTS
		}
	case queue.HandlerThreadUpdate:
		if update, ok := entry.Payload.(*models.ThreadUpdatePartial); ok && update.UpdatedTS != nil {
			return *update.UpdatedTS
		}
	case queue.HandlerThreadDelete:
		// For delete, TS is not in payload, but since no sorting issue, return 0
		return 0
	case queue.HandlerMessageCreate:
		if m, ok := entry.Payload.(*models.Message); ok {
			return m.TS
		}
	case queue.HandlerMessageUpdate:
		if update, ok := entry.Payload.(*models.MessageUpdatePartial); ok && update.TS != nil {
			return *update.TS
		}
	case queue.HandlerMessageDelete:
		if del, ok := entry.Payload.(*models.DeletePartial); ok {
			return del.TS
		}

	}
	return 0
}

func extractAuthor(entry types.BatchEntry) string {
	switch entry.Handler {
	case queue.HandlerThreadCreate:
		if th, ok := entry.Payload.(*models.Thread); ok {
			return th.Author
		}
	case queue.HandlerThreadUpdate:
		// For update, author not in payload, but since op.Extras.UserID was used, but not available, perhaps return ""
		return ""
	case queue.HandlerThreadDelete:
		// For delete, author not in payload, return ""
		return ""
	case queue.HandlerMessageCreate:
		if m, ok := entry.Payload.(*models.Message); ok {
			return m.Author
		}
	case queue.HandlerMessageUpdate:
		// For update, author not in payload, return ""
		return ""
	case queue.HandlerMessageDelete:
		if del, ok := entry.Payload.(*models.DeletePartial); ok {
			return del.Author
		}

	}
	return ""
}

func extractTID(entry types.BatchEntry) string {
	switch entry.Handler {
	case queue.HandlerThreadCreate:
		if th, ok := entry.Payload.(*models.Thread); ok {
			return th.ID
		}
	case queue.HandlerThreadUpdate:
		if update, ok := entry.Payload.(*models.ThreadUpdatePartial); ok && update.ID != nil {
			return *update.ID
		}
	case queue.HandlerThreadDelete:
		if del, ok := entry.Payload.(*models.ThreadDeletePartial); ok && del.ID != nil {
			return *del.ID
		}
	case queue.HandlerMessageCreate:
		if m, ok := entry.Payload.(*models.Message); ok {
			return m.Thread
		}
	case queue.HandlerMessageUpdate:
		if update, ok := entry.Payload.(*models.MessageUpdatePartial); ok && update.Thread != nil {
			return *update.Thread
		}
	case queue.HandlerMessageDelete:
		if del, ok := entry.Payload.(*models.DeletePartial); ok {
			return del.Thread
		}
	}
	return ""
}

func extractMID(entry types.BatchEntry) string {
	switch entry.Handler {
	case queue.HandlerThreadCreate, queue.HandlerThreadUpdate, queue.HandlerThreadDelete:
		return ""
	case queue.HandlerMessageCreate:
		if m, ok := entry.Payload.(*models.Message); ok {
			return m.ID
		}
	case queue.HandlerMessageUpdate:
		if update, ok := entry.Payload.(*models.MessageUpdatePartial); ok && update.ID != nil {
			return *update.ID
		}
	case queue.HandlerMessageDelete:
		if del, ok := entry.Payload.(*models.DeletePartial); ok {
			return del.ID
		}
	}
	return ""
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
		return extractTS(sorted[i]) < extractTS(sorted[j])
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

// collectProvisionalMessageKeys extracts all provisional message keys from batch entries
func collectProvisionalMessageKeys(entries []types.BatchEntry) []string {
	provKeyMap := make(map[string]bool)

	for _, entry := range entries {
		// Check message ID in payload
		if entry.Payload != nil {
			if msg, ok := entry.Payload.(*models.Message); ok && msg.ID != "" && keys.IsProvisionalMessageKey(msg.ID) {
				provKeyMap[msg.ID] = true
			}
		}
	}

	provKeys := make([]string, 0, len(provKeyMap))
	for provKey := range provKeyMap {
		provKeys = append(provKeys, provKey)
	}
	return provKeys
}

// bulkLookupProvisionalKeys looks up multiple provisional keys in database and returns existing mappings
func bulkLookupProvisionalKeys(provKeys []string) (map[string]string, error) {
	mappings := make(map[string]string)

	if storedb.Client == nil {
		logger.Debug("store_not_ready_for_bulk_lookup")
		return mappings, nil
	}

	iter, err := storedb.Client.NewIter(nil)
	if err != nil {
		logger.Error("bulk_lookup_iterator_failed", "error", err)
		return mappings, err
	}
	defer iter.Close()

	for _, provKey := range provKeys {
		// Create prefix for provisional key + ":" to find the sequenced key
		prefix := provKey + ":"

		// Seek to the prefix
		iter.SeekGE([]byte(prefix))

		if iter.Valid() && len(iter.Key()) > len(prefix) && string(iter.Key()[:len(prefix)]) == prefix {
			// Found the actual sequenced key
			finalKey := string(iter.Key())
			mappings[provKey] = finalKey
			logger.Debug("bulk_lookup_found_mapping", "provisional", provKey, "final", finalKey)
		} else {
			logger.Debug("bulk_lookup_not_found", "provisional", provKey)
		}
	}

	return mappings, nil
}

// prepopulateProvisionalCache loads existing provisional->final mappings into MessageSequencer cache
func prepopulateProvisionalCache(batchProcessor *BatchProcessor, mappings map[string]string) {
	batchProcessor.Index.PrepopulateProvisionalCache(mappings)
}

func ApplyBatchToDB(entries []types.BatchEntry) error {
	tr := telemetry.Track("ingest.apply_batch")
	defer tr.Finish()
	if len(entries) == 0 {
		return nil
	}
	logger.Debug("batch_apply_start", "entries", len(entries))
	tr.Mark("group_operations")
	batchProcessor := NewBatchProcessor()

	// put reqs into thread groups
	// each request by its thread_id parent
	threadGroups := groupByThread(entries)
	logger.Debug("batch_grouped", "threads", len(threadGroups))
	for threadID, threadEntries := range threadGroups {
		logger.Debug("thread_group", "thread_id", threadID, "operations", len(threadEntries))
		for _, entry := range threadEntries {
			logger.Debug("thread_group_op", "thread_id", threadID, "handler", entry.Handler, "tid", entry.TID, "mid", entry.MID)
		}
	}

	// initialize thread sequences from database
	// load up the sequencing info per threads (or init anew)
	threadIDs := collectThreadIDsFromGroups(threadGroups)
	if len(threadIDs) > 0 {
		tr.Mark("init_thread_sequences")
		if err := batchProcessor.Index.InitializeThreadSequencesFromDB(threadIDs); err != nil {
			logger.Error("init_thread_sequences_failed", "err", err)
		}
	}

	// pre-load provisional key mappings from database
	//
	provKeys := collectProvisionalMessageKeys(entries)
	if len(provKeys) > 0 {
		tr.Mark("preload_provisional_keys")
		mappings, err := bulkLookupProvisionalKeys(provKeys)
		if err != nil {
			logger.Error("preload_provisional_keys_failed", "err", err)
		} else {
			prepopulateProvisionalCache(batchProcessor, mappings)
			logger.Debug("preload_provisional_keys_complete", "total_keys", len(provKeys), "found_mappings", len(mappings))
		}
	}
	tr.Mark("process_thread_groups")

	for threadID, threadEntries := range threadGroups {
		sortedOps := sortOperationsByType(threadEntries)
		logger.Debug("batch_processing_thread", "thread", threadID, "ops", len(sortedOps))
		for _, op := range sortedOps {
			if err := BProcOperation(op, batchProcessor); err != nil {
				logger.Error("process_operation_failed", "err", err, "handler", op.Handler, "thread", op.TID, "msg", op.MID)
				continue
			}
		}
	}
	tr.Mark("flush_batch")
	logger.Debug("batch_flush_start")
	if err := batchProcessor.Flush(); err != nil {
		logger.Error("batch_flush_failed", "err", err)
		return fmt.Errorf("batch flush failed: %w", err)
	}
	logger.Info("batch_applied", "entries", len(entries))
	tr.Mark("record_write")
	storedb.RecordWrite(len(entries))
	return nil
}
