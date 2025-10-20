package ingest

import (
	"context"
	"encoding/json"

	"progressdb/pkg/ingest/queue"
	"progressdb/pkg/logger"
	"progressdb/pkg/models"
	"progressdb/pkg/store/messages"
	"progressdb/pkg/store/threads"
	"progressdb/pkg/telemetry"
)

// ApplyBatchToDB persists a list of BatchEntry items to the storedb.
// Message entries are saved via storedb.SaveMessage (handles encryption and sequencing).
// Thread entries are processed with SaveThread or SoftDeleteThread as appropriate.
func ApplyBatchToDB(entries []BatchEntry) error {
	tr := telemetry.Track("ingest.apply_batch")
	defer tr.Finish()

	if len(entries) == 0 {
		return nil
	}

	tr.Mark("process_entries")
	for _, e := range entries {
		switch {
		case e.MsgID != "":
			// Message entry: unmarshal and save.
			var msg models.Message
			if err := json.Unmarshal(e.Payload, &msg); err != nil {
				logger.Error("apply_batch_unmarshal_message", "err", err)
				continue
			}
			if err := messages.SaveMessage(context.Background(), e.Thread, e.MsgID, msg); err != nil {
				logger.Error("apply_batch_save_message_failed", "err", err, "thread", e.Thread, "msg", e.MsgID)
				continue
			}
		default:
			// Thread-level entry. Use SoftDeleteThread for deletes, otherwise SaveThread.
			if e.Handler == queue.HandlerThreadDelete {
				if err := threads.SoftDeleteThread(e.Thread, ""); err != nil {
					logger.Error("apply_batch_soft_delete_failed", "err", err, "thread", e.Thread)
					continue
				}
			} else {
				if err := threads.SaveThread(e.Thread, string(e.Payload)); err != nil {
					logger.Error("apply_batch_save_thread_failed", "err", err, "thread", e.Thread)
					continue
				}
			}
		}
	}
	tr.Mark("record_write")
	// storedb.RecordWrite(len(entries))
	return nil
}
