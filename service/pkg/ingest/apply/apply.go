package apply

import (
	"fmt"
	"sort"

	"progressdb/pkg/ingest/queue"
	"progressdb/pkg/ingest/types"
	"progressdb/pkg/models"
	storedb "progressdb/pkg/store/db/store"
	"progressdb/pkg/store/keys"
)

func groupOperationsByThreadKey(entries []types.BatchEntry) map[string][]types.BatchEntry {
	threadGroups := make(map[string][]types.BatchEntry)
	for _, entry := range entries {
		threadKey := ExtractTKey(entry.QueueOp)
		threadGroups[threadKey] = append(threadGroups[threadKey], entry)
	}
	return threadGroups
}

func getOperationPriority(handler queue.HandlerID) int {
	switch handler {
	case queue.HandlerThreadCreate, queue.HandlerThreadUpdate:
		return 1
	case queue.HandlerMessageCreate, queue.HandlerMessageUpdate:
		return 2
	case queue.HandlerThreadDelete, queue.HandlerMessageDelete:
		return 3
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
		if update, ok := entry.Payload.(*models.ThreadUpdatePartial); ok && update.UpdatedTS != 0 {
			return update.UpdatedTS
		}
	case queue.HandlerThreadDelete:
		return 0
	case queue.HandlerMessageCreate:
		if m, ok := entry.Payload.(*models.Message); ok {
			return m.TS
		}
	case queue.HandlerMessageUpdate:
		if update, ok := entry.Payload.(*models.MessageUpdatePartial); ok && update.TS != 0 {
			return update.TS
		}
	case queue.HandlerMessageDelete:
		if del, ok := entry.Payload.(*models.MessageDeletePartial); ok {
			return del.TS
		}
	}

	// this is not going to happen
	// but if if by magic it occurs
	// - crash the system (to prevent any blind ops)
	panic("extractTS: unsupported operation or handler")
}

func extractAuthor(entry types.BatchEntry) string {
	switch entry.Handler {
	case queue.HandlerThreadCreate:
		if th, ok := entry.Payload.(*models.Thread); ok {
			return th.Author
		}
	case queue.HandlerThreadUpdate:
		return entry.QueueOp.Extras.UserID
	case queue.HandlerThreadDelete:
		return entry.QueueOp.Extras.UserID
	case queue.HandlerMessageCreate:
		if m, ok := entry.Payload.(*models.Message); ok {
			return m.Author
		}
	case queue.HandlerMessageUpdate:
		return entry.QueueOp.Extras.UserID
	case queue.HandlerMessageDelete:
		if del, ok := entry.Payload.(*models.MessageDeletePartial); ok {
			return del.Author
		}
	}

	// this is not going to happen
	// but if if by magic it occurs
	// - crash the system (to prevent any blind ops)
	panic("extractAuthor: unsupported operation or handler")
}

func ExtractTKey(qop *queue.QueueOp) string {
	switch qop.Handler {
	case queue.HandlerThreadCreate:
		if th, ok := qop.Payload.(*models.Thread); ok {
			return th.Key
		}
	case queue.HandlerThreadUpdate:
		if update, ok := qop.Payload.(*models.ThreadUpdatePartial); ok && update.Key != "" {
			return update.Key
		}
	case queue.HandlerThreadDelete:
		if del, ok := qop.Payload.(*models.ThreadDeletePartial); ok && del.Key != "" {
			return del.Key
		}
	case queue.HandlerMessageCreate:
		if msg, ok := qop.Payload.(*models.Message); ok {
			return msg.Thread
		}
	case queue.HandlerMessageUpdate:
		if update, ok := qop.Payload.(*models.MessageUpdatePartial); ok && update.Thread != "" {
			return update.Thread
		}
	case queue.HandlerMessageDelete:
		if del, ok := qop.Payload.(*models.MessageDeletePartial); ok && del.Thread != "" {
			return del.Thread
		}
	}

	// this is not going to happen
	// but if if by magic it occurs
	// - crash the system (to prevent any blind ops)
	panic("ExtractTKey: unsupported operation or handler")
}

func ExtractMKey(qop *queue.QueueOp) string {
	switch qop.Handler {
	case queue.HandlerThreadCreate, queue.HandlerThreadUpdate, queue.HandlerThreadDelete:
		return ""
	case queue.HandlerMessageCreate:
		if m, ok := qop.Payload.(*models.Message); ok {
			return m.Key
		}
	case queue.HandlerMessageUpdate:
		if update, ok := qop.Payload.(*models.MessageUpdatePartial); ok && update.Key != "" {
			return update.Key
		}
	case queue.HandlerMessageDelete:
		if del, ok := qop.Payload.(*models.MessageDeletePartial); ok {
			return del.Key
		}
	}

	// this is not going to happen
	// but if if by magic it occurs
	// - crash the system (to prevent any blind ops)
	panic("ExtractMKey: unsupported operation or handler")
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

func extractUniqueThreadKeys(threadGroups map[string][]types.BatchEntry) []string {
	// unique existence
	threadMap := make(map[string]bool)
	for threadKey := range threadGroups {
		if threadKey != "" {
			threadMap[threadKey] = true
		}
	}
	threadKeys := make([]string, 0, len(threadMap))
	for threadKey := range threadMap {
		threadKeys = append(threadKeys, threadKey)
	}
	return threadKeys
}

func collectProvisionalMessageKeys(entries []types.BatchEntry) []string {
	provKeyMap := make(map[string]bool)
	for _, entry := range entries {
		if entry.Payload != nil {
			if msg, ok := entry.Payload.(*models.Message); ok && msg.Key != "" && keys.IsProvisionalMessageKey(msg.Key) {
				provKeyMap[msg.Key] = true
			}
		}
	}
	provKeys := make([]string, 0, len(provKeyMap))
	for provKey := range provKeyMap {
		provKeys = append(provKeys, provKey)
	}
	return provKeys
}

func bulkLookupProvisionalKeys(provKeys []string) (map[string]string, error) {
	mappings := make(map[string]string)
	if storedb.Client == nil {
		return mappings, nil
	}
	iter, err := storedb.Client.NewIter(nil)
	if err != nil {
		return mappings, err
	}
	defer iter.Close()
	for _, provKey := range provKeys {
		prefix := provKey + ":"
		iter.SeekGE([]byte(prefix))
		if iter.Valid() && len(iter.Key()) > len(prefix) && string(iter.Key()[:len(prefix)]) == prefix {
			finalKey := string(iter.Key())
			mappings[provKey] = finalKey
		}
	}
	return mappings, nil
}

func prepopulateProvisionalCache(batchProcessor *BatchProcessor, mappings map[string]string) {
	batchProcessor.Index.PrepopulateProvisionalCache(mappings)
}

func ApplyBatchToDB(entries []types.BatchEntry) error {
	if len(entries) == 0 {
		return nil
	}

	// has the store and index clients
	batchProcessor := NewBatchProcessor()

	// for us to preload or preinit threads
	threadGroups := groupOperationsByThreadKey(entries)

	// get thread keys unique list
	threadKeys := extractUniqueThreadKeys(threadGroups)

	// setup the thread states
	if len(threadKeys) > 0 {
		_ = batchProcessor.Index.InitializeThreadSequencesFromDB(threadKeys)
	}

	//
	provKeys := collectProvisionalMessageKeys(entries)
	if len(provKeys) > 0 {
		mappings, _ := bulkLookupProvisionalKeys(provKeys)
		prepopulateProvisionalCache(batchProcessor, mappings)
	}
	for _, threadEntries := range threadGroups {
		sortedOps := sortOperationsByType(threadEntries)
		for _, op := range sortedOps {
			_ = BProcOperation(op, batchProcessor)
		}
	}
	if err := batchProcessor.Flush(); err != nil {
		return fmt.Errorf("batch flush failed: %w", err)
	}
	storedb.RecordWrite(len(entries))
	return nil
}
