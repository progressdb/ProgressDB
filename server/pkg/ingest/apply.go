package ingest

import (
	"context"
	"encoding/json"

	"progressdb/pkg/ingest/queue"
	"progressdb/pkg/logger"
	"progressdb/pkg/models"
	"progressdb/pkg/store"
)

// applyBatchToDB converts BatchEntry list into persisted store actions.
// For message entries we call store.SaveMessage to ensure encryption and
// per-thread sequencing logic is applied. For thread entries we call the
// appropriate thread store helpers (SaveThread / SoftDeleteThread).
func applyBatchToDB(entries []BatchEntry) error {
	if len(entries) == 0 {
		return nil
	}

	// Apply each entry using the store helpers so we preserve thread-level
	// semantics (encryption, LastSeq bump, version indexing).
	for _, e := range entries {
		switch {
		case e.MsgID != "":
			// message create/update/delete: unmarshal payload and call SaveMessage
			var msg models.Message
			if err := json.Unmarshal(e.Payload, &msg); err != nil {
				logger.Error("apply_batch_unmarshal_message", "err", err)
				continue
			}
			if err := store.SaveMessage(context.Background(), e.Thread, e.MsgID, msg); err != nil {
				logger.Error("apply_batch_save_message_failed", "err", err, "thread", e.Thread, "msg", e.MsgID)
				// continue processing remaining entries
				continue
			}
		default:
			// thread-level entry (MsgID empty). Interpret OpDelete as soft-delete
			// otherwise write thread metadata.
			if e.Handler == queue.HandlerThreadDelete {
				// attempt soft-delete (actor unknown here)
				if err := store.SoftDeleteThread(e.Thread, ""); err != nil {
					logger.Error("apply_batch_soft_delete_failed", "err", err, "thread", e.Thread)
					continue
				}
			} else {
				// write thread meta
				if err := store.SaveThread(e.Thread, string(e.Payload)); err != nil {
					logger.Error("apply_batch_save_thread_failed", "err", err, "thread", e.Thread)
					continue
				}
			}
		}
	}
	// record writes for accounting
	store.RecordWrite(len(entries))
	return nil
}
