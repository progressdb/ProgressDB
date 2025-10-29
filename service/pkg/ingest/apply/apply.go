package apply

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"progressdb/pkg/ingest/queue"
	"progressdb/pkg/ingest/types"
	"progressdb/pkg/models"
	"progressdb/pkg/state"
	storedb "progressdb/pkg/store/db/store"
	"progressdb/pkg/store/keys"

	"github.com/cockroachdb/pebble"
)

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
	index.messageSequencer = sequencer // set the sequencer
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

func getOperationPriority(handler queue.HandlerID) int {
	switch handler {
	case queue.HandlerThreadCreate:
		return 1
	case queue.HandlerThreadUpdate:
		return 2
	case queue.HandlerThreadDelete:
		return 3
	case queue.HandlerMessageCreate:
		return 4
	case queue.HandlerMessageUpdate:
		return 5
	case queue.HandlerMessageDelete:
		return 6
	}

	// This should never happen.
	state.Crash("get_operation_priority_failed", fmt.Errorf("getOperationPriority: unsupported handler type: %v", handler))
	return 0
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
	state.Crash("index_state_init_failed", fmt.Errorf("extractTS: unsupported operation or handler"))
	return 0
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
	state.Crash("index_state_init_failed", fmt.Errorf("extractAuthor: unsupported operation or handler"))
	return ""
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
	state.Crash("index_state_init_failed", fmt.Errorf("ExtractTKey: unsupported operation or handler"))
	return ""
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
	provKeyMap := make(map[string]bool) // dedup set
	for _, entry := range entries {
		// only *models.Message with provisional key
		if msg, ok := entry.Payload.(*models.Message); ok && msg.Key != "" && keys.IsProvisionalMessageKey(msg.Key) {
			provKeyMap[msg.Key] = true
		}
	}
	provKeys := make([]string, 0, len(provKeyMap)) // result list
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
	// Group provKeys by thread for efficient bounded iteration
	threadToProvKeys := make(map[string][]string)
	for _, provKey := range provKeys {
		parts := strings.Split(provKey, ":")
		if len(parts) >= 4 && parts[0] == "t" && parts[2] == "m" {
			thread := parts[1]
			threadToProvKeys[thread] = append(threadToProvKeys[thread], provKey)
		}
	}
	// Iterate per thread with bounds
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

// nextPrefix computes the next lexicographic key after a given prefix
func nextPrefix(prefix []byte) []byte {
	out := make([]byte, len(prefix))
	copy(out, prefix)
	for i := len(out) - 1; i >= 0; i-- {
		if out[i] < 0xFF {
			out[i]++
			return out[:i+1]
		}
	}
	return nil // no upper bound if all 0xFF
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

	// prov - references to async keys
	// loadup any final key mappings
	provKeys := collectProvisionalMessageKeys(entries)
	if len(provKeys) > 0 {
		mappings, _ := bulkLookupProvisionalKeys(provKeys)
		// cache
		batchProcessor.Index.PrepopulateProvisionalCache(mappings)
	}

	// process each thread & its ops
	for _, threadEntries := range threadGroups {
		// ops sorted by create, update, delete
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
