package apply

import (
	"bytes"
	"fmt"
	"sort"

	"progressdb/pkg/ingest/tracking"
	"progressdb/pkg/ingest/types"
	"progressdb/pkg/models"
	"progressdb/pkg/state"
	"progressdb/pkg/state/logger"
	"progressdb/pkg/store/db/storedb"
	"progressdb/pkg/store/keys"

	"github.com/cockroachdb/pebble"
)

var globalWAL types.WAL

func SetIntakeWAL(wal types.WAL) {
	globalWAL = wal
}

func truncateWALWithSequences(seqs []uint64) error {
	if globalWAL == nil || len(seqs) == 0 {
		return nil
	}
	if walWithTruncate, ok := globalWAL.(interface{ TruncateSequences([]uint64) error }); ok {
		if err := walWithTruncate.TruncateSequences(seqs); err != nil {
			return fmt.Errorf("wal truncate sequences failed: %w", err)
		}
		logger.Debug("wal_truncated_batch", "seq_count", len(seqs))
	}
	return nil
}

type BatchProcessor struct {
	KV        *KVManager
	Index     *IndexManager
	Data      *DataManager
	Sequencer *MessageSequencer
}

func NewBatchProcessor() *BatchProcessor {
	kv := NewKVManager()
	index := NewIndexManager(kv)
	data := NewDataManager(kv)
	sequencer := NewMessageSequencer(index, kv)
	index.messageSequencer = sequencer
	return &BatchProcessor{
		KV:        kv,
		Index:     index,
		Data:      data,
		Sequencer: sequencer,
	}
}

func (bp *BatchProcessor) Flush() error {
	return bp.KV.Flush()
}

func groupOperationsByThreadKey(entries []types.BatchEntry) map[string][]types.BatchEntry {
	threadGroups := make(map[string][]types.BatchEntry)
	for _, entry := range entries {
		threadKey := ExtractTKey(entry.QueueOp)
		threadGroups[threadKey] = append(threadGroups[threadKey], entry)
	}
	return threadGroups
}

func getOperationPriority(handler types.HandlerID) int {
	switch handler {
	case types.HandlerThreadCreate:
		return 1
	case types.HandlerThreadUpdate:
		return 2
	case types.HandlerThreadDelete:
		return 3
	case types.HandlerMessageCreate:
		return 4
	case types.HandlerMessageUpdate:
		return 5
	case types.HandlerMessageDelete:
		return 6
	default:
		state.Crash("get_operation_priority_failed", fmt.Errorf("getOperationPriority: unsupported handler type: %v", handler))
		return 10
	}
}

func extractTS(entry types.BatchEntry) int64 {
	switch entry.Handler {
	case types.HandlerThreadCreate:
		if th, ok := entry.Payload.(*models.Thread); ok {
			return th.CreatedTS
		}
	case types.HandlerThreadUpdate:
		if update, ok := entry.Payload.(*models.ThreadUpdatePartial); ok && update.UpdatedTS != 0 {
			return update.UpdatedTS
		}
	case types.HandlerThreadDelete:
		return 0
	case types.HandlerMessageCreate:
		if m, ok := entry.Payload.(*models.Message); ok {
			return m.CreatedTS
		}
	case types.HandlerMessageUpdate:
		if update, ok := entry.Payload.(*models.MessageUpdatePartial); ok && update.UpdatedTS != 0 {
			return update.UpdatedTS
		}
	case types.HandlerMessageDelete:
		if del, ok := entry.Payload.(*models.MessageDeletePartial); ok {
			return del.UpdatedTS
		}
	}

	state.Crash("index_state_init_failed", fmt.Errorf("extractTS: unsupported operation or handler"))
	return 0
}

func extractAuthor(entry types.BatchEntry) string {
	switch entry.Handler {
	case types.HandlerThreadCreate:
		if th, ok := entry.Payload.(*models.Thread); ok {
			return th.Author
		}
	case types.HandlerThreadUpdate:
		return entry.QueueOp.Extras.UserID
	case types.HandlerThreadDelete:
		return entry.QueueOp.Extras.UserID
	case types.HandlerMessageCreate:
		if m, ok := entry.Payload.(*models.Message); ok {
			return m.Author
		}
	case types.HandlerMessageUpdate:
		return entry.QueueOp.Extras.UserID
	case types.HandlerMessageDelete:
		if del, ok := entry.Payload.(*models.MessageDeletePartial); ok {
			return del.Author
		}
	}

	state.Crash("index_state_init_failed", fmt.Errorf("extractAuthor: unsupported operation or handler"))
	return ""
}

func ExtractTKey(qop *types.QueueOp) string {
	switch qop.Handler {
	case types.HandlerThreadCreate:
		if th, ok := qop.Payload.(*models.Thread); ok {
			return th.Key
		}
	case types.HandlerThreadUpdate:
		if update, ok := qop.Payload.(*models.ThreadUpdatePartial); ok && update.Key != "" {
			return update.Key
		}
	case types.HandlerThreadDelete:
		if del, ok := qop.Payload.(*models.ThreadDeletePartial); ok && del.Key != "" {
			return del.Key
		}
	case types.HandlerMessageCreate:
		if msg, ok := qop.Payload.(*models.Message); ok {
			return msg.Thread
		}
	case types.HandlerMessageUpdate:
		if update, ok := qop.Payload.(*models.MessageUpdatePartial); ok && update.Thread != "" {
			return update.Thread
		}
	case types.HandlerMessageDelete:
		if del, ok := qop.Payload.(*models.MessageDeletePartial); ok && del.Thread != "" {
			return del.Thread
		}
	}

	state.Crash("index_state_init_failed", fmt.Errorf("ExtractTKey: unsupported operation or handler"))
	return ""
}

func ExtractMKey(qop *types.QueueOp) string {
	switch qop.Handler {
	case types.HandlerThreadCreate, types.HandlerThreadUpdate, types.HandlerThreadDelete:
		return ""
	case types.HandlerMessageCreate:
		if m, ok := qop.Payload.(*models.Message); ok {
			return m.Key
		}
	case types.HandlerMessageUpdate:
		if update, ok := qop.Payload.(*models.MessageUpdatePartial); ok && update.Key != "" {
			return update.Key
		}
	case types.HandlerMessageDelete:
		if del, ok := qop.Payload.(*models.MessageDeletePartial); ok {
			return del.Key
		}
	}

	state.Crash("index_state_init_failed", fmt.Errorf("ExtractMKey: unsupported operation or handler"))
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

func extractUniqueThreadKeys(threadGroups map[string][]types.BatchEntry) []string {
	seen := make(map[string]bool)
	var result []string
	for threadKey := range threadGroups {
		if threadKey != "" && !seen[threadKey] {
			seen[threadKey] = true
			result = append(result, threadKey)
		}
	}
	return result
}

func collectProvisionalMessageKeys(entries []types.BatchEntry) []string {
	provKeyMap := make(map[string]bool)
	for _, entry := range entries {
		if msg, ok := entry.Payload.(*models.Message); ok && msg.Key != "" {
			if parsed, err := keys.ParseKey(msg.Key); err == nil && parsed.Type == keys.KeyTypeMessageProvisional {
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
	threadToProvKeys := make(map[string][]string)
	for _, provKey := range provKeys {
		if parsed, err := keys.ParseKey(provKey); err == nil && parsed.Type == keys.KeyTypeMessageProvisional {
			threadToProvKeys[parsed.ThreadKey] = append(threadToProvKeys[parsed.ThreadKey], provKey)
		}
	}
	for thread, keys := range threadToProvKeys {
		prefix := []byte("t:" + thread + ":m:")
		upper := nextPrefix(prefix)
		iter, err := storedb.Client.NewIter(&pebble.IterOptions{
			LowerBound: prefix,
			UpperBound: upper,
		})
		if err != nil {
			return mappings, err
		}
		for _, provKey := range keys {
			seekKey := []byte(provKey + ":")
			iter.SeekGE(seekKey)
			if iter.Valid() && bytes.HasPrefix(iter.Key(), seekKey) {
				mappings[provKey] = string(iter.Key())
			}
		}
		iter.Close()
	}
	return mappings, nil
}

func nextPrefix(prefix []byte) []byte {
	out := make([]byte, len(prefix))
	copy(out, prefix)
	for i := len(out) - 1; i >= 0; i-- {
		if out[i] < 0xFF {
			out[i]++
			return out[:i+1]
		}
	}
	return nil
}

func extractSequencesFromBatch(entries []types.BatchEntry) []uint64 {
	seqs := make([]uint64, 0, len(entries))
	for _, entry := range entries {
		if entry.Enq > 0 {
			seqs = append(seqs, entry.Enq)
		}
	}
	return seqs
}

func ApplyBatchToDB(entries []types.BatchEntry) error {
	if len(entries) == 0 {
		return nil
	}

	batchProcessor := NewBatchProcessor()

	threadGroups := groupOperationsByThreadKey(entries)

	uniqueThreadKeys := extractUniqueThreadKeys(threadGroups)

	if len(uniqueThreadKeys) > 0 {
		if err := batchProcessor.Index.InitializeThreadSequencesFromDB(uniqueThreadKeys); err != nil {
			logger.Error("thread_sequence_init_failed", "err", err, "thread_count", len(uniqueThreadKeys))
		}
	}

	provKeys := collectProvisionalMessageKeys(entries)
	if len(provKeys) > 0 {
		mappings, err := bulkLookupProvisionalKeys(provKeys)
		if err != nil {
			logger.Error("bulk_key_lookup_failed", "err", err, "key_count", len(provKeys))
		} else {
			tracking.GlobalKeyMapper.PrepopulateBatchCache(mappings)
		}
	}

	// process per thread groupings
	for _, threadEntries := range threadGroups {
		sortedOps := sortOperationsByType(threadEntries)
		for _, op := range sortedOps {
			if err := BProcOperation(op, batchProcessor); err != nil {
				logger.Error("operation_processing_failed", "err", err, "handler", op.Handler)
			}
		}
	}

	// commit to database
	if err := batchProcessor.Flush(); err != nil {
		return fmt.Errorf("batch flush failed: %w", err)
	}

	// purge trackers
	inflightKeys := collectInflightKeys(entries)
	removeFromInflightTracking(inflightKeys)

	// purge current batch lookup mappings
	tracking.GlobalKeyMapper.ClearBatchCache()

	// wal truncates if needed
	seqs := extractSequencesFromBatch(entries)
	if err := truncateWALWithSequences(seqs); err != nil {
		logger.Error("wal_truncate_failed", "err", err, "seq_count", len(seqs))
	}

	return nil
}

func collectInflightKeys(entries []types.BatchEntry) []string {
	var inflightKeys []string
	for _, entry := range entries {
		switch entry.Handler {
		case types.HandlerThreadCreate:
			if thread, ok := entry.Payload.(*models.Thread); ok && thread.Key != "" {
				inflightKeys = append(inflightKeys, thread.Key)
			}
		case types.HandlerMessageCreate:
			if msg, ok := entry.Payload.(*models.Message); ok && msg.Key != "" {
				inflightKeys = append(inflightKeys, msg.Key)
			}
		}
	}
	return inflightKeys
}

func removeFromInflightTracking(provisionalKeys []string) {
	for _, key := range provisionalKeys {
		tracking.GlobalInflightTracker.Remove(key)
	}
}
