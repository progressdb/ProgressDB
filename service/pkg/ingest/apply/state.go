package apply

import (
	"encoding/json"
	"fmt"

	"progressdb/pkg/state/logger"
	"progressdb/pkg/store/keys"
)

type BatchProcessor struct {
	Index *IndexManager // Public field access
	Data  *DataManager  // Public field access
	KV    *KVManager    // Centralized KV cache
}

func NewBatchProcessor() *BatchProcessor {
	kv := NewKVManager()
	return &BatchProcessor{
		Index: NewIndexManager(kv),
		Data:  NewDataManager(kv),
		KV:    kv,
	}
}

func (bp *BatchProcessor) Flush() error {
	// Serialize thread messages to KV
	threadMessages := bp.Index.GetThreadMessages()
	for threadID, threadIdx := range threadMessages {
		if threadIdx == nil {
			// Delete all index keys for this thread
			suffixes := []string{"start", "end", "cdeltas", "udeltas", "skips", "last_created_at", "last_updated_at"}
			for _, suffix := range suffixes {
				var key string
				switch suffix {
				case "start":
					key = keys.GenThreadMessageStart(threadID)
				case "end":
					key = keys.GenThreadMessageEnd(threadID)
				case "cdeltas":
					key = keys.GenThreadMessageCDeltas(threadID)
				case "udeltas":
					key = keys.GenThreadMessageUDeltas(threadID)
				case "skips":
					key = keys.GenThreadMessageSkips(threadID)
				case "last_created_at":
					key = keys.GenThreadMessageLC(threadID)
				case "last_updated_at":
					key = keys.GenThreadMessageLU(threadID)
				}
				bp.KV.SetIndexKV(key, nil)
			}
		} else {
			// Save each field to its respective key
			fields := map[string]interface{}{
				"start":           threadIdx.Start,
				"end":             threadIdx.End,
				"cdeltas":         threadIdx.Cdeltas,
				"udeltas":         threadIdx.Udeltas,
				"skips":           threadIdx.Skips,
				"last_created_at": threadIdx.LastCreatedAt,
				"last_updated_at": threadIdx.LastUpdatedAt,
			}

			for suffix, val := range fields {
				var key string
				switch suffix {
				case "start":
					key = keys.GenThreadMessageStart(threadID)
				case "end":
					key = keys.GenThreadMessageEnd(threadID)
				case "cdeltas":
					key = keys.GenThreadMessageCDeltas(threadID)
				case "udeltas":
					key = keys.GenThreadMessageUDeltas(threadID)
				case "skips":
					key = keys.GenThreadMessageSkips(threadID)
				case "last_created_at":
					key = keys.GenThreadMessageLC(threadID)
				case "last_updated_at":
					key = keys.GenThreadMessageLU(threadID)
				}
				data, err := json.Marshal(val)
				if err != nil {
					logger.Error("marshal thread index", "suffix", suffix, "threadID", threadID, "err", err)
					continue
				}
				bp.KV.SetIndexKV(key, data)
			}
		}
	}

	// Flush all KV changes
	if err := bp.KV.Flush(); err != nil {
		logger.Error("kv_flush_error", "err", err)
		return fmt.Errorf("kv flush: %w", err)
	}

	// Reset after successful flush
	bp.Reset()
	logger.Debug("batch_reset_complete")

	return nil
}

// Reset clears all accumulated changes after batch completion
func (bp *BatchProcessor) Reset() {
	bp.Index.Reset()
	bp.KV.Reset()
}
